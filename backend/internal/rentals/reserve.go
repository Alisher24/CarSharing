package rentals

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReserveCommand asks to hold one vehicle for one account.
type ReserveCommand struct {
	Caller    uuid.UUID
	VehicleID string
	Attempt   Attempt
}

// Reserve holds one available vehicle for the reservation lifetime, or answers why it cannot.
//
// The whole decision is one transaction: the accounts and vehicles involved are locked in the shared
// order, the relationships are read again under those locks, and only then are the day's allowance,
// the caller's own rental and the state of the chosen vehicle judged. A command that decides nothing
// still commits its answer, because the refusal is what a repeat of the same command must reproduce.
func (s *Service) Reserve(ctx context.Context, command ReserveCommand) (Answered, error) {
	return s.answer(ctx, idempotency.ForAccount(command.Caller), command.Attempt,
		reserveParticipants(s.pool, command),
		func(ctx context.Context, tx pgx.Tx, moment time.Time) (Outcome, error) {
			return s.reservationWithin(ctx, tx, moment, command)
		})
}

// answer runs one command: it either replays the answer the key already holds, or decides the
// command, renders its answer and stores that answer under the key. The claim, the decision and the
// stored answer are one transaction, so a change that is committed is a change whose answer can
// never be lost, and an answer that is stored describes a change that happened.
func (s *Service) answer(
	ctx context.Context,
	owner idempotency.Owner,
	attempt Attempt,
	discover func(context.Context) (participants, error),
	decide func(context.Context, pgx.Tx, time.Time) (Outcome, error),
) (Answered, error) {
	var answered Answered
	err := transact(ctx, s.pool, discover, func(txCtx context.Context, tx pgx.Tx, moment time.Time) error {
		claim, err := idempotency.ClaimKey(txCtx, tx, owner, attempt.Key, attempt.Fingerprint)
		if err != nil {
			return err
		}
		if !claim.Owns {
			answered = Answered{Response: Response(claim.Answered), Replayed: true}
			return nil
		}

		decided, err := decide(txCtx, tx, moment)
		if err != nil {
			return err
		}
		response, err := attempt.Render(decided)
		if err != nil {
			return err
		}
		stored := idempotency.Result{Status: response.Status, Body: response.Body}
		if err := idempotency.Complete(txCtx, tx, owner, attempt.Key, stored); err != nil {
			return err
		}
		answered = Answered{Response: response}
		return nil
	})
	if err != nil {
		return Answered{}, err
	}
	return answered, nil
}

// reserveParticipants is the rows a reservation touches: the caller, the chosen vehicle, the holder
// of that vehicle when somebody has it, and the rentals that currently hold either of them.
func reserveParticipants(pool *pgxpool.Pool, command ReserveCommand) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		planned := participants{
			users:    []uuid.UUID{command.Caller},
			vehicles: []string{command.VehicleID},
		}
		held, err := liveRentalOf(ctx, pool, userLiveRentalSelection, command.Caller)
		if err != nil {
			return participants{}, err
		}
		taken, err := liveRentalOf(ctx, pool, vehicleLiveRentalSelection, command.VehicleID)
		if err != nil {
			return participants{}, err
		}
		if taken != nil {
			planned.users = append(planned.users, taken.UserID)
		}
		planned.users = distinctUsers(planned.users)
		planned.rentals = sortedIdentifiers(identifierOf(held), identifierOf(taken))
		return planned, nil
	}
}

// reservationWithin decides the command with the participants locked and the moment fixed.
func (s *Service) reservationWithin(
	ctx context.Context,
	tx pgx.Tx,
	moment time.Time,
	command ReserveCommand,
) (Outcome, error) {
	held, err := s.releaseOverdueReservations(ctx, tx, moment, command)
	if err != nil {
		return Outcome{}, err
	}
	refusal, err := s.accountReservationRefusal(ctx, command.Caller, moment, held)
	if err != nil {
		return Outcome{}, err
	}
	if refusal != nil {
		return refused(moment, *refusal), nil
	}
	vehicle, refusal, err := s.reservableVehicle(ctx, command.VehicleID, moment)
	if err != nil {
		return Outcome{}, err
	}
	if refusal != nil {
		return refused(moment, *refusal), nil
	}
	return s.createReservation(ctx, tx, moment, command, vehicle)
}

// releaseOverdueReservations updates both live relationships before the command judges either one.
func (s *Service) releaseOverdueReservations(
	ctx context.Context,
	tx pgx.Tx,
	moment time.Time,
	command ReserveCommand,
) (*Rental, error) {
	held, err := liveRentalAt(
		ctx,
		s.pool,
		s.vehicles,
		s.warnings,
		tx,
		moment,
		userLiveRentalSelection,
		command.Caller,
	)
	if err != nil {
		return nil, err
	}
	_, err = liveRentalAt(
		ctx,
		s.pool,
		s.vehicles,
		s.warnings,
		tx,
		moment,
		vehicleLiveRentalSelection,
		command.VehicleID,
	)
	if err != nil {
		return nil, err
	}
	return held, nil
}

// accountReservationRefusal applies account rules in the order useful to the caller.
func (s *Service) accountReservationRefusal(
	ctx context.Context,
	caller uuid.UUID,
	moment time.Time,
	held *Rental,
) (*Refusal, error) {
	owed, err := s.invoices.Outstanding(ctx, caller)
	if err != nil {
		return nil, err
	}
	if owed {
		return &Refusal{Kind: OutstandingInvoice}, nil
	}

	limit, err := readDailyLimit(ctx, s.pool, caller, moment)
	if err != nil {
		return nil, err
	}
	if !limit.Available {
		return &Refusal{Kind: DailyLimitReached, Limit: limit}, nil
	}
	if held != nil {
		return &Refusal{Kind: ActiveRentalExists}, nil
	}
	return nil, nil
}

func (s *Service) reservableVehicle(
	ctx context.Context,
	vehicleID string,
	moment time.Time,
) (fleet.Vehicle, *Refusal, error) {
	vehicle, err := s.vehicles.VehicleAt(ctx, vehicleID, moment)
	if errors.Is(err, fleet.ErrVehicleNotFound) {
		return fleet.Vehicle{}, &Refusal{Kind: VehicleUnavailable}, nil
	}
	if err != nil {
		return fleet.Vehicle{}, nil, err
	}
	return vehicle, vehicleRefusal(vehicle, moment), nil
}

func (s *Service) createReservation(
	ctx context.Context,
	tx pgx.Tx,
	moment time.Time,
	command ReserveCommand,
	vehicle fleet.Vehicle,
) (Outcome, error) {
	price, err := s.prices.InForce(ctx)
	if err != nil {
		return Outcome{}, err
	}
	rental, reserved, err := insertReservation(ctx, s.pool, reservation{
		userID:    command.Caller,
		vehicleID: command.VehicleID,
		zoneID:    vehicle.ServiceZoneID,
		price:     price,
		moment:    moment,
	})
	if err != nil {
		return Outcome{}, err
	}
	if !reserved {
		return refused(moment, contendedRefusal(ctx, s.pool, command)), nil
	}

	raised, err := s.vehicles.PublishChange(ctx, tx, command.VehicleID, fleet.VehicleChange{})
	if err != nil {
		return Outcome{}, err
	}
	if err = events.Record(ctx, s.pool,
		events.Signal{Kind: events.RentalChanged, ResourceID: rental.ID,
			Version: rental.Version, Recipient: command.Caller},
		events.Signal{Kind: events.VehicleChanged, ResourceID: command.VehicleID, Version: raised},
	); err != nil {
		return Outcome{}, err
	}
	published, err := s.vehicles.VehicleAt(ctx, command.VehicleID, moment)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Rental: rental, Vehicle: published, Moment: moment}, nil
}

// vehicleRefusal reports why a vehicle may not be reserved at an instant, or nil when it may. A
// vehicle somebody holds is refused before its own condition is judged: how full it is says nothing
// about whether it is free.
func vehicleRefusal(vehicle fleet.Vehicle, moment time.Time) *Refusal {
	if vehicle.HeldBy.Holds() {
		return &Refusal{Kind: VehicleUnavailable}
	}
	state := vehicle.StateAt(moment)
	if state.Status != fleet.Available {
		return &Refusal{
			Kind:               VehicleUnavailable,
			UnavailableReasons: state.UnavailableReasons,
		}
	}
	return nil
}

// contendedRefusal names which live rental kept the row out: the caller's own or the vehicle's.
func contendedRefusal(ctx context.Context, pool *pgxpool.Pool, command ReserveCommand) Refusal {
	held, err := liveRentalOf(ctx, pool, userLiveRentalSelection, command.Caller)
	if err == nil && held != nil {
		return Refusal{Kind: ActiveRentalExists}
	}
	return Refusal{Kind: VehicleUnavailable}
}

func refused(moment time.Time, refusal Refusal) Outcome {
	return Outcome{Moment: moment, Refusal: refusal}
}

// reservation is one reservation about to be written.
type reservation struct {
	userID    uuid.UUID
	vehicleID string
	zoneID    string
	price     tariffs.Tariff
	moment    time.Time
}

// distinctUsers orders the accounts of a transaction the way the lock statement reads them.
func distinctUsers(users []uuid.UUID) []uuid.UUID {
	sort.Slice(users, func(left, right int) bool {
		return users[left].String() < users[right].String()
	})
	return withoutRepeats(users)
}

// sortedIdentifiers orders the rental identifiers of one transaction, leaving out the rentals that
// are absent. The lock statement reads them in this order, so two transactions touching the same
// rentals wait in the same sequence.
func sortedIdentifiers(identifiers ...string) []string {
	kept := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		if identifier != "" {
			kept = append(kept, identifier)
		}
	}
	sort.Strings(kept)
	return withoutRepeats(kept)
}

// identifierOf names a rental that may be absent, so a caller can list the rentals of a transaction
// without testing each one.
func identifierOf(rental *Rental) string {
	if rental == nil {
		return ""
	}
	return rental.ID
}

// withoutRepeats drops the neighbours that are equal. It requires ordered input, so repeats are
// adjacent, and reuses the backing array because the ordered slice is not read afterwards.
func withoutRepeats[T comparable](rows []T) []T {
	unique := rows[:0]
	for _, row := range rows {
		if len(unique) > 0 && unique[len(unique)-1] == row {
			continue
		}
		unique = append(unique, row)
	}
	return unique
}
