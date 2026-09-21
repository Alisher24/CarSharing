package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RideKind is one command that moves a ride from the stage it is in to the next one.
type RideKind string

const (
	// StartRide turns a reservation into a ride that is driving.
	StartRide RideKind = "start"

	// PauseRide makes a driving ride stand still.
	PauseRide RideKind = "pause"

	// ResumeRide makes a paused ride drive again.
	ResumeRide RideKind = "resume"
)

// RideCommand names one rental to move along the ride lifecycle, whoever asks. Whether the caller may
// move it is decided from the rental the module reads rather than from anything the request carries.
type RideCommand struct {
	Caller   uuid.UUID
	RentalID string
	Attempt  Attempt
}

// Ride moves one of the caller's rentals along the ride lifecycle, or answers why it cannot.
//
// The whole decision is one transaction under the shared lock order: the rental, its vehicle and the
// account are locked, the relationships are read again under those locks, and the moment every
// boundary is judged by is read from the database afterwards. A start that meets a reservation whose
// deadline has passed records the expiry before it answers, and a refusal never undoes a transition
// that had already become due.
func (s *Service) Ride(ctx context.Context, kind RideKind, command RideCommand) (Answered, error) {
	transition, known := rideTransitions[kind]
	if !known {
		return Answered{}, errors.New("the ride command is not one the module knows")
	}
	return s.answer(ctx, idempotency.ForAccount(command.Caller), command.Attempt,
		rideParticipants(s.pool, command),
		func(ctx context.Context, tx pgx.Tx, moment time.Time) (Outcome, error) {
			return s.rideWithin(ctx, tx, transition, moment, command)
		})
}

// rideTransition is what one command requires and what it produces. The rule is stated as data rather
// than as a condition per command, so the stage a command applies to and the stage it reaches stand
// together and a new command is a new row.
type rideTransition struct {
	from stage.Stage
	to   stage.Stage

	// mode is the mode the ride is in after the move, which is the interval the transition opens.
	mode Mode
}

// rideTransitions is the whole of "which command applies where", including the stage each one
// produces. Start applies to a reservation, and pausing and continuing apply to a ride that has begun,
// so a command met in any other stage is a refusal rather than a transition.
var rideTransitions = map[RideKind]rideTransition{
	StartRide:  {from: stage.Reserved, to: stage.Active, mode: Driving},
	PauseRide:  {from: stage.Active, to: stage.Paused, mode: Paused},
	ResumeRide: {from: stage.Paused, to: stage.Active, mode: Driving},
}

// rideParticipants is the rows a ride command touches: the caller, the rental it names, the vehicle
// that rental holds, and whichever rental currently holds either of them.
func rideParticipants(pool *pgxpool.Pool, command RideCommand) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		planned := participants{users: []uuid.UUID{command.Caller}}
		target, err := rentalByIDFor(ctx, pool, command.Caller, command.RentalID)
		if errors.Is(err, ErrRentalNotFound) {
			// An identifier that names no rental of this account is answered the same way whether the
			// rental does not exist or belongs to somebody else, so nothing else is locked.
			return planned, nil
		}
		if err != nil {
			return participants{}, err
		}
		planned.vehicles = []string{target.VehicleID}
		planned.rentals = sortedIdentifiers(target.ID)
		return planned, nil
	}
}

// rideWithin decides one ride command with the participants locked and the moment fixed.
func (s *Service) rideWithin(
	ctx context.Context,
	tx pgx.Tx,
	transition rideTransition,
	moment time.Time,
	command RideCommand,
) (Outcome, error) {
	target, refusal, err := s.reconciledRideTarget(ctx, tx, moment, command)
	if err != nil {
		return Outcome{}, err
	}
	if refusal != nil {
		return refused(moment, *refusal), nil
	}

	if target.Overdue(moment) {
		return s.recordExpiry(ctx, tx, moment, target)
	}
	if refusal := rideRefusal(target, transition); refusal != nil {
		return refused(moment, *refusal), nil
	}
	refusal, err = s.ridePrepared(ctx, transition, moment, target)
	if err != nil {
		return Outcome{}, err
	}
	if refusal != nil {
		return refused(moment, *refusal), nil
	}
	return s.movePreparedRide(ctx, tx, transition, moment, target)
}

func (s *Service) reconciledRideTarget(
	ctx context.Context,
	tx pgx.Tx,
	moment time.Time,
	command RideCommand,
) (Rental, *Refusal, error) {
	target, err := rentalByIDFor(ctx, s.pool, command.Caller, command.RentalID)
	if errors.Is(err, ErrRentalNotFound) {
		return Rental{}, &Refusal{Kind: RentalNotFound}, nil
	}
	if err != nil {
		return Rental{}, nil, err
	}
	if _, err = s.reconcileVehicle(ctx, tx, moment, target.VehicleID, &target); err != nil {
		return Rental{}, nil, err
	}
	target, err = rentalByIDFor(ctx, s.pool, command.Caller, command.RentalID)
	if err != nil {
		return Rental{}, nil, err
	}
	return target, nil, nil
}

func (s *Service) movePreparedRide(
	ctx context.Context,
	tx pgx.Tx,
	transition rideTransition,
	moment time.Time,
	target Rental,
) (Outcome, error) {
	moved, err := moveRide(ctx, s.pool, target, transition, moment)
	if err != nil {
		return Outcome{}, err
	}
	if err = beginSegment(ctx, s.pool, target.ID, transition.mode, moment); err != nil {
		return Outcome{}, err
	}
	if err = deactivateWarning(ctx, s.warnings, moved); err != nil {
		return Outcome{}, err
	}
	if err = announceRide(ctx, s.pool, s.vehicles, tx, moved); err != nil {
		return Outcome{}, err
	}
	vehicle, err := s.vehicles.VehicleAt(ctx, moved.VehicleID, moment)
	if err != nil {
		return Outcome{}, err
	}
	progress, err := readProgress(ctx, s.pool, moved, moment)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Rental: moved, Vehicle: vehicle, Moment: moment, Progress: progress}, nil
}

// rideRefusal reports why the command does not move this rental, or nil when it does. A rental that
// another command has already moved answers the stage it reached rather than the one the caller
// expected, so a repeated pause is refused instead of opening a second interval.
func rideRefusal(target Rental, transition rideTransition) *Refusal {
	if target.Stage == transition.from {
		return nil
	}
	switch target.Stage {
	case stage.Cancelled, stage.Expired:
		return &Refusal{Kind: ReservationExpired}
	case stage.Completed:
		return &Refusal{Kind: RentalCompleted}
	default:
		return &Refusal{Kind: InvalidRentalState}
	}
}

// ridePrepared reports what a command requires of the vehicle before it may move the rental. Only a
// start has such a condition: a ride that has begun continues on whatever is left, so pausing and
// continuing are never refused for the state of an energy source.
func (s *Service) ridePrepared(
	ctx context.Context, transition rideTransition, moment time.Time, target Rental,
) (*Refusal, error) {
	if transition.from != stage.Reserved {
		return nil, nil
	}
	vehicle, err := s.vehicles.VehicleAt(ctx, target.VehicleID, moment)
	if err != nil {
		return nil, err
	}
	if vehicle.FitToStart() {
		return nil, nil
	}
	return &Refusal{Kind: VehicleUnavailable, UnavailableReasons: vehicle.StartRefusalReasons()}, nil
}

// announceRide raises the version of the vehicle the ride holds and records the signals of both
// changes, so the catalog a visitor reads and the account that holds the ride hear about the mode it
// entered in the transaction that entered it.
func announceRide(
	ctx context.Context,
	pool *pgxpool.Pool,
	vehicles *fleet.Store,
	tx pgx.Tx,
	moved Rental,
) error {
	version, err := vehicles.PublishChange(ctx, tx, moved.VehicleID, fleet.VehicleChange{})
	if err != nil {
		return err
	}
	return events.Record(ctx, pool,
		events.Signal{Kind: events.RentalChanged, ResourceID: moved.ID,
			Version: moved.Version, Recipient: moved.UserID},
		events.Signal{Kind: events.VehicleChanged, ResourceID: moved.VehicleID, Version: version},
	)
}
