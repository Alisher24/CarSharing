package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// reservationSweep is the recurring pass over the reservations of the fleet. Which reservations it
// takes and what it does with each of them are the two things a caller supplies: the release of a
// reservation past its deadline and the warning of one inside its last minute are one path, differing
// in the transition they write.
type reservationSweep struct {
	pool     *pgxpool.Pool
	vehicles *fleet.Store
	warnings WarningOperations
}

func newReservationSweep(
	pool *pgxpool.Pool,
	vehicles *fleet.Store,
	warnings WarningOperations,
) *reservationSweep {
	return &reservationSweep{pool: pool, vehicles: vehicles, warnings: warnings}
}

// transition is what a sweep does to one reservation it found due. It reports whether this sweep is the
// one that moved it: a reservation another transaction moved first is left alone.
type transition func(ctx context.Context, tx pgx.Tx, moment time.Time, due Rental) (bool, error)

// sweepDue moves every reservation the statement finds due and reports how many it moved.
func (s *reservationSweep) sweepDue(
	ctx context.Context,
	due func(context.Context, *pgxpool.Pool) ([]string, error),
	move transition,
) (int64, error) {
	found, err := due(ctx, s.pool)
	if err != nil {
		return 0, err
	}
	var moved int64
	for _, id := range found {
		one, err := s.moveOne(ctx, id, move)
		if err != nil {
			return moved, err
		}
		if one {
			moved++
		}
	}
	return moved, nil
}

// moveOne moves one reservation that was due when the sweep read it, in its own transaction under the
// shared lock order. A sweep and a command arriving at the same moment therefore wait for each other
// rather than taking the accounts and vehicles of the fleet in two different orders.
//
// The rental is read again after the locks, and the transition judges it as it stands at the moment the
// transaction fixed: a reservation that has left the stage the transition applies to in the meantime is
// left alone, and a sweep that arrives late does not move it a second time.
func (s *reservationSweep) moveOne(
	ctx context.Context, id string, move transition,
) (bool, error) {
	var moved bool
	err := transact(ctx, s.pool, rentalParticipants(s.pool, id),
		func(txCtx context.Context, tx pgx.Tx, moment time.Time) error {
			due, err := rentalByID(txCtx, s.pool, id)
			if errors.Is(err, ErrRentalNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			moved, err = move(txCtx, tx, moment, due)
			return err
		})
	return moved, err
}
