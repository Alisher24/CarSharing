package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpirySweepInterval is how often reservations are checked against their deadlines. A reservation
// is therefore released within a second of running out, which is close enough for a person
// watching the map to see the vehicle free itself.
const ExpirySweepInterval = time.Second

// Expiry ends reservations whose deadline has passed. It is the rentals module's own transition: the
// catalog reads the result rather than depicting a release the database has not made.
type Expiry struct{ pool *pgxpool.Pool }

func NewExpiry(pool *pgxpool.Pool) *Expiry { return &Expiry{pool: pool} }

// ExpireDue ends every reservation already past its deadline and reports how many it ended. The
// rental ends at the deadline itself rather than at the moment this ran, so a late sweep does not
// extend a reservation that had already run out.
//
// Ending a reservation changes what the fleet publishes and what its holder is told, so the version
// of the rental and of its vehicle are raised and the signals of both are recorded in the same
// transaction as the transition: a release nobody is told about would leave two clients showing a
// vehicle as taken.
func (e *Expiry) ExpireDue(ctx context.Context) (int64, error) {
	var ended int64
	err := database.InTransaction(ctx, e.pool, func(txCtx context.Context) error {
		due, err := dueReservations(txCtx, e.pool)
		if err != nil {
			return err
		}
		signals := make([]events.Signal, 0, len(due)*2)
		for _, reservation := range due {
			version, err := expireReservation(txCtx, e.pool, reservation.id)
			if err != nil {
				return err
			}
			vehicleVersion, err := raiseVehicleVersion(txCtx, e.pool, reservation.vehicleID)
			if err != nil {
				return err
			}
			signals = append(signals,
				events.Signal{Kind: events.RentalChanged, ResourceID: reservation.id,
					Version: version, Recipient: reservation.userID},
				events.Signal{Kind: events.VehicleChanged, ResourceID: reservation.vehicleID,
					Version: vehicleVersion},
			)
		}
		ended = int64(len(due))
		return events.Record(txCtx, e.pool, signals...)
	})
	return ended, err
}

// reservation is a reservation that has run out, resolved to what ending it needs.
type reservation struct {
	id        string
	vehicleID string
	userID    uuid.UUID
}

// dueReservations takes the reservations that have run out and locks them. The lock is what makes two
// sweeps arriving together end each reservation once: the second one waits, then finds the row in a
// stage that no longer matches and leaves it alone.
const dueReservationsStatement = `
SELECT id, vehicle_id, user_id
FROM rentals
WHERE stage = $1 AND expires_at <= now()
ORDER BY expires_at, id
FOR UPDATE`

func dueReservations(ctx context.Context, pool *pgxpool.Pool) ([]reservation, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, dueReservationsStatement, Reserved)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var due []reservation
	for rows.Next() {
		var current reservation
		if err = rows.Scan(&current.id, &current.vehicleID, &current.userID); err != nil {
			return nil, err
		}
		due = append(due, current)
	}
	return due, rows.Err()
}

// expireReservation ends one reservation at its own deadline and returns the version it reached.
const expireReservationStatement = `
UPDATE rentals
SET stage = $1, ended_at = expires_at, version = version + 1
WHERE id = $2 AND stage = $3
RETURNING version`

func expireReservation(ctx context.Context, pool *pgxpool.Pool, id string) (int64, error) {
	var version int64
	err := database.QuerierFrom(ctx, pool).QueryRow(ctx, expireReservationStatement,
		Expired, id, Reserved).Scan(&version)
	return version, err
}

// raiseVehicleVersion marks the vehicle as changed for every reader of the catalog and returns the
// version it reached. A version is raised rather than set, because a client compares the version of
// two readings to decide which one is newer.
const raiseVehicleVersionStatement = `
UPDATE vehicles SET version = version + 1 WHERE id = $1 RETURNING version`

func raiseVehicleVersion(ctx context.Context, pool *pgxpool.Pool, vehicleID string) (int64, error) {
	var version int64
	err := database.QuerierFrom(ctx, pool).QueryRow(ctx, raiseVehicleVersionStatement, vehicleID).
		Scan(&version)
	return version, err
}
