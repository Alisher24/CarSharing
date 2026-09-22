package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeadlineSweepInterval is how often reservations are checked against their deadlines. A reservation
// is therefore released, and its warning created, within a second of the moment that decides it,
// which is close enough for a person watching the map to see the vehicle free itself.
const DeadlineSweepInterval = time.Second

// dueReservations names the reservations whose deadline has passed, which is what the sweep hands to
// the expiry transition. The moment is the database's own, so the pass and the command that meets the
// same reservation judge it by one clock.
func dueReservations(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, dueReservationsStatement, stage.Reserved)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var due []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		due = append(due, id)
	}
	return due, rows.Err()
}

// expireDue ends every reservation already past its deadline and reports how many it ended.
func (s *reservationSweep) expireDue(ctx context.Context) (int64, error) {
	return s.sweepDue(ctx, dueReservations, s.expire)
}

// expire ends one reservation that was due when the sweep read it. The deadline is compared again with
// the moment the transaction fixed after its locks, because the wait for those locks may have been
// long: a reservation that is not due at that moment is left for a later sweep.
func (s *reservationSweep) expire(
	ctx context.Context, tx pgx.Tx, moment time.Time, due Rental,
) (bool, error) {
	if !due.Overdue(moment) {
		return false, nil
	}
	return endReservation(ctx, s.pool, s.vehicles, s.warnings, tx, due)
}

// liveRentalAt reads the rental that currently holds one account or one vehicle at the moment the
// transaction fixed, ending it first when its deadline has been reached.
//
// An overdue reservation is not live for any path: whoever meets one — a command, the read of what
// is current — releases it here and decides its own subject afterwards, in the same transaction. The
// release is therefore committed with that decision, and a domain refusal does not undo it, which is
// what lets another account take a vehicle the fleet has not swept yet.
func liveRentalAt(
	ctx context.Context,
	pool *pgxpool.Pool,
	vehicles *fleet.Store,
	warnings WarningOperations,
	tx pgx.Tx,
	moment time.Time,
	selection string,
	identifier any,
) (*Rental, error) {
	held, err := liveRentalOf(ctx, pool, selection, identifier)
	if err != nil || held == nil {
		return nil, err
	}
	if !held.Overdue(moment) {
		return held, nil
	}
	if _, err = endReservation(ctx, pool, vehicles, warnings, tx, *held); err != nil {
		return nil, err
	}
	return nil, nil
}
