package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Mode is what a vehicle does during one interval of a ride. The values are the ones the contract
// publishes as ride_mode, and the catalog shows the mode of the interval that is open.
type Mode string

const (
	Driving Mode = "driving"
	Paused  Mode = "paused"
)

// closeOpenSegment ends the interval the ride is in at the given moment. The pairing of the end and
// the absence of one is the whole of "still open", so a ride another transaction has already moved on
// is left alone: exactly one interval per transition is closed, and never an already closed one.
const closeOpenSegmentStatement = `
UPDATE ride_segments
SET ended_at = $2
WHERE rental_id = $1 AND ended_at IS NULL`

func closeOpenSegment(ctx context.Context, pool *pgxpool.Pool, rentalID string, moment time.Time) error {
	_, err := database.QuerierFrom(ctx, pool).Exec(ctx, closeOpenSegmentStatement, rentalID, moment)
	return err
}

// openSegment starts a new interval of a ride at the given moment. The partial unique index on the
// open interval is what refuses a second one, so no caller reads before it writes.
const openSegmentStatement = `
INSERT INTO ride_segments (rental_id, mode, started_at) VALUES ($1, $2, $3)`

func openSegment(ctx context.Context, pool *pgxpool.Pool, rentalID string, mode Mode, moment time.Time) error {
	_, err := database.QuerierFrom(ctx, pool).Exec(ctx, openSegmentStatement, rentalID, mode, moment)
	return err
}

// beginSegment closes the interval a ride is in and opens the next one at the same moment, which is
// what makes the boundary between two intervals one instant with neither a gap nor an overlap. A ride
// that has not started yet has nothing to close.
//
// Both statements run in the transaction that moved the rental and holds its lock, so no reader sees a
// ride with no open interval or with two.
func beginSegment(
	ctx context.Context, pool *pgxpool.Pool, rentalID string, mode Mode, moment time.Time,
) error {
	if err := closeOpenSegment(ctx, pool, rentalID, moment); err != nil {
		return err
	}
	return openSegment(ctx, pool, rentalID, mode, moment)
}
