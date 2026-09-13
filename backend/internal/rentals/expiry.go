package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpirySweepInterval is how often reservations are checked against their deadlines. A reservation
// is therefore released within a second of running out, which is close enough for a person watching
// the map to see the vehicle free itself.
const ExpirySweepInterval = time.Second

// Expiry ends reservations whose deadline has passed. It is the rentals module's own transition: the
// catalog reads the result rather than depicting a release the database has not made.
type Expiry struct{ pool *pgxpool.Pool }

func NewExpiry(pool *pgxpool.Pool) *Expiry { return &Expiry{pool: pool} }

// dueReservations finds the reservations that have run out. The selection takes no lock: a
// reservation an equally timed sweep or a command has already ended since is left alone by the
// transition below, which is where the decision is made rather than here.
const dueReservationsStatement = `
SELECT id
FROM rentals
WHERE stage = $1 AND expires_at <= clock_timestamp()
ORDER BY expires_at, id`

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

// ExpireDue ends every reservation already past its deadline and reports how many it ended.
//
// Each reservation is ended in its own transaction under the shared lock order, so a sweep and a
// command arriving at the same moment wait for each other rather than taking the accounts and
// vehicles of the fleet in two different orders. A reservation another transaction ended first is
// counted as not ended by this sweep: the transition happened once.
func (e *Expiry) ExpireDue(ctx context.Context) (int64, error) {
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
func (e *Expiry) expire(ctx context.Context, id string) (bool, error) {
	var ended bool
	err := transact(ctx, e.pool, expiryParticipants(e.pool, id),
		func(txCtx context.Context, moment time.Time) error {
			due, err := rentalByID(txCtx, e.pool, id)
			if errors.Is(err, ErrRentalNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if due.Stage != stage.Reserved || !due.Overdue(moment) {
				return nil
			}
			ended, err = endReservation(txCtx, e.pool, due)
			return err
		})
	return ended, err
}

// expiryParticipants is the rows ending one reservation touches: the account that holds it, its
// vehicle, and the rental itself.
func expiryParticipants(pool *pgxpool.Pool, id string) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		due, err := rentalByID(ctx, pool, id)
		if errors.Is(err, ErrRentalNotFound) {
			return participants{}, nil
		}
		if err != nil {
			return participants{}, err
		}
		return participants{
			users:    []uuid.UUID{due.UserID},
			vehicles: []string{due.VehicleID},
			rentals:  []string{due.ID},
		}, nil
	}
}
