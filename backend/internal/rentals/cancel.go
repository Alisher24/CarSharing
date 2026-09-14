package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CancelCommand gives one reservation back.
type CancelCommand struct {
	Caller   uuid.UUID
	RentalID string
	Attempt  Attempt
}

// Cancel gives a reservation back before its deadline, or reports why it cannot.
//
// The deadline decides the answer, and it is compared with the authoritative moment the transaction
// fixed after its waits rather than with the moment the request arrived. A cancellation that arrives
// after the deadline records the expiry first and answers that the reservation has run out: a domain
// refusal never undoes a transition that had already become due.
func (s *Service) Cancel(ctx context.Context, command CancelCommand) (Answered, error) {
	return s.answer(ctx, command.Caller, command.Attempt,
		cancelParticipants(s.pool, command),
		func(ctx context.Context, moment time.Time) (Outcome, error) {
			return s.cancellationWithin(ctx, moment, command)
		})
}

// cancelParticipants is the rows a cancellation touches: the caller, the rental it names together
// with the vehicle that rental holds, and whichever rental currently holds either of them.
func cancelParticipants(pool *pgxpool.Pool, command CancelCommand) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		planned := participants{users: []uuid.UUID{command.Caller}}
		target, err := rentalByIDFor(ctx, pool, command.Caller, command.RentalID)
		if errors.Is(err, ErrRentalNotFound) {
			// Nothing else belongs to this transaction: an identifier that names no rental of this
			// account is answered the same way whether the rental does not exist or belongs to
			// somebody else.
			return planned, nil
		}
		if err != nil {
			return participants{}, err
		}
		planned.vehicles = []string{target.VehicleID}
		held, err := liveRentalOf(ctx, pool, userLiveRentalSelection, command.Caller)
		if err != nil {
			return participants{}, err
		}
		taken, err := liveRentalOf(ctx, pool, vehicleLiveRentalSelection, target.VehicleID)
		if err != nil {
			return participants{}, err
		}
		if taken != nil {
			planned.users = append(planned.users, taken.UserID)
		}
		planned.users = distinctUsers(planned.users)
		planned.rentals = sortedIdentifiers(target.ID, identifierOf(held), identifierOf(taken))
		return planned, nil
	}
}

// cancellationWithin decides the command with the participants locked and the moment fixed.
func (s *Service) cancellationWithin(
	ctx context.Context, moment time.Time, command CancelCommand,
) (Outcome, error) {
	target, err := rentalByIDFor(ctx, s.pool, command.Caller, command.RentalID)
	if errors.Is(err, ErrRentalNotFound) {
		return refused(moment, Refusal{Kind: RentalNotFound}), nil
	}
	if err != nil {
		return Outcome{}, err
	}

	switch target.Stage {
	case stage.Reserved:
		if target.Overdue(moment) {
			return s.recordExpiry(ctx, moment, target)
		}
		return s.cancelWithin(ctx, moment, target)
	case stage.Expired:
		return refused(moment, Refusal{Kind: ReservationExpired}), nil
	case stage.Completed:
		return refused(moment, Refusal{Kind: RentalCompleted}), nil
	default:
		return refused(moment, Refusal{Kind: InvalidRentalState}), nil
	}
}

// cancelWithin moves one reservation to cancelled and announces the change. A cancelled reservation
// leaves no billable interval, no invoice and no message behind, and it does not return the day's
// allowance: that was spent when the reservation was made.
func (s *Service) cancelWithin(ctx context.Context, moment time.Time, target Rental) (Outcome, error) {
	cancelled, moved, err := endReservationAs(ctx, s.pool, target.ID, stage.Cancelled, moment)
	if err != nil {
		return Outcome{}, err
	}
	if !moved {
		// The rental row this transaction locked is no longer a reservation, which the lock it took
		// makes unreachable; answering it as a stage the command does not apply to is the honest
		// answer rather than reporting a cancellation that did not happen.
		return refused(moment, Refusal{Kind: InvalidRentalState}), nil
	}
	// The reservation has left the reserved stage, so its warning stops being current in the same
	// transaction that moved it.
	if err = deactivateWarning(ctx, s.pool, cancelled); err != nil {
		return Outcome{}, err
	}
	if err = announceEnd(ctx, s.pool, target, cancelled); err != nil {
		return Outcome{}, err
	}
	vehicle, err := s.vehicles.VehicleAt(ctx, target.VehicleID, moment)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Rental: cancelled, Vehicle: vehicle, Moment: moment}, nil
}

// recordExpiry ends a reservation whose deadline the command discovered, then answers that it has
// run out. The transition is written before the answer and committed with it, so a client told that
// the reservation has expired is told about a release that actually happened.
func (s *Service) recordExpiry(ctx context.Context, moment time.Time, target Rental) (Outcome, error) {
	if _, err := endReservation(ctx, s.pool, target); err != nil {
		return Outcome{}, err
	}
	return refused(moment, Refusal{Kind: ReservationExpired}), nil
}

// endReservationAs moves one reservation to a stage that releases its vehicle and returns the rental
// as it now stands. The stage it moves from is part of the statement, so a rental another
// transaction has already moved is left alone and reported as unmoved.
const releaseRentalStatement = `
UPDATE rentals
SET stage = $3, ended_at = $4, mode_started_at = NULL, version = version + 1
WHERE id = $1 AND stage = $2
RETURNING` + rentalFields

func endReservationAs(
	ctx context.Context, pool *pgxpool.Pool, id string, ending stage.Stage, at time.Time,
) (Rental, bool, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, releaseRentalStatement,
		id, stage.Reserved, ending, at)
	if err != nil {
		return Rental{}, false, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Rental{}, false, err
		}
		return Rental{}, false, nil
	}
	var ended Rental
	if err = scanRental(rows, &ended); err != nil {
		return Rental{}, false, err
	}
	return ended, true, rows.Err()
}

// endReservation ends one reservation at its own deadline, releases its vehicle, deactivates its
// warning and announces all three changes. It is the single implementation of the transition: the
// sweep, the read that discovers a deadline and the command that arrives after one all reach it.
//
// The rental ends at its deadline rather than at the moment this ran, so a sweep that arrives late
// does not extend a reservation that had already run out. A reservation another transaction has
// already ended is left alone and reported as not ended here.
func endReservation(ctx context.Context, pool *pgxpool.Pool, due Rental) (bool, error) {
	ended, moved, err := endReservationAs(ctx, pool, due.ID, stage.Expired, due.ExpiresAt)
	if err != nil || !moved {
		return false, err
	}
	if err = deactivateWarning(ctx, pool, ended); err != nil {
		return false, err
	}
	return true, announceEnd(ctx, pool, due, ended)
}

// announceEnd raises the version of the vehicle a rental has released and records the signals of
// both changes, so the fleet a visitor reads and the account that held the rental hear about the
// release in the same transaction that made it.
func announceEnd(ctx context.Context, pool *pgxpool.Pool, held, released Rental) error {
	version, err := raiseVehicleVersion(ctx, pool, held.VehicleID)
	if err != nil {
		return err
	}
	return events.Record(ctx, pool,
		events.Signal{Kind: events.RentalChanged, ResourceID: released.ID,
			Version: released.Version, Recipient: released.UserID},
		events.Signal{Kind: events.VehicleChanged, ResourceID: held.VehicleID, Version: version},
	)
}
