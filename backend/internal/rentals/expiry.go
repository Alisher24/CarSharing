package rentals

import (
	"context"
	"errors"
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

// expiry ends reservations whose deadline has passed. It is the rentals module's own transition: the
// catalog reads the result rather than depicting a release the database has not made.
type expiry struct {
	pool     *pgxpool.Pool
	vehicles *fleet.Store
	warnings WarningOperations
}

func newExpiry(
	pool *pgxpool.Pool,
	vehicles *fleet.Store,
	warnings WarningOperations,
) *expiry {
	return &expiry{pool: pool, vehicles: vehicles, warnings: warnings}
}

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
//
// Each reservation is ended in its own transaction under the shared lock order, so a sweep and a
// command arriving at the same moment wait for each other rather than taking the accounts and
// vehicles of the fleet in two different orders. A reservation another transaction ended first is
// counted as not ended by this sweep: the transition happened once.
func (e *expiry) expireDue(ctx context.Context) (int64, error) {
	due, err := dueReservations(ctx, e.pool)
	if err != nil {
		return 0, err
	}
	var ended int64
	for _, id := range due {
		moved, err := e.expire(ctx, id)
		if err != nil {
			return ended, err
		}
		if moved {
			ended++
		}
	}
	return ended, nil
}

// expire ends one reservation that was due when the sweep read it. The deadline is compared again
// after the locks with the moment the transaction fixed, because the wait for those locks may have
// been long: a reservation that is not due at that moment is left for a later sweep.
func (e *expiry) expire(ctx context.Context, id string) (bool, error) {
	var ended bool
	err := transact(ctx, e.pool, rentalParticipants(e.pool, id),
		func(txCtx context.Context, tx pgx.Tx, moment time.Time) error {
			due, err := rentalByID(txCtx, e.pool, id)
			if errors.Is(err, ErrRentalNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if !due.Overdue(moment) {
				return nil
			}
			ended, err = endReservation(txCtx, e.pool, e.vehicles, e.warnings, tx, due)
			return err
		})
	return ended, err
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
