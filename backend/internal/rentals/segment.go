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

func closeOpenSegment(ctx context.Context, pool *pgxpool.Pool, rentalID string, moment time.Time) error {
	_, err := database.QuerierFrom(ctx, pool).Exec(ctx, closeOpenSegmentStatement, rentalID, moment)
	return err
}

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
