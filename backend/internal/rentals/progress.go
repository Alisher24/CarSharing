package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Progress is what a live ride publishes about the time it has taken: the durations of each mode, the
// minutes begun in each and what those minutes cost. It is computed at one moment from the intervals
// of the ride, so nothing here can disagree with the intervals the database holds.
type Progress struct {
	DrivingDuration time.Duration
	PausedDuration  time.Duration
	DrivingMinutes  int64
	PausedMinutes   int64
	AmountTyiyn     int64
}

// startedMinutesStatement applies the billing policy to the intervals of one ride: each mode is summed
// over the whole ride first, and each sum is rounded up once. Rounding every interval separately would
// bill three twenty-second intervals as three begun minutes instead of one.
//
// The sum reads the interval the ride is in up to the answered moment, and no interval that began after
// it, so a read taken before a transition never counts time the transition has not made yet.
const startedMinutesStatement = `
SELECT mode,
       COALESCE(sum(COALESCE(ended_at, $2::timestamptz) - started_at), '0'::interval),
       COALESCE(ceil(EXTRACT(EPOCH FROM sum(COALESCE(ended_at, $2::timestamptz) - started_at)) / 60), 0)::bigint
FROM ride_segments
WHERE rental_id = $1 AND started_at <= $2
GROUP BY mode`

func readProgress(
	ctx context.Context, pool *pgxpool.Pool, rental Rental, moment time.Time,
) (Progress, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, startedMinutesStatement, rental.ID, moment)
	if err != nil {
		return Progress{}, err
	}
	defer rows.Close()

	var progress Progress
	for rows.Next() {
		var (
			mode     Mode
			duration time.Duration
			minutes  int64
		)
		if err = rows.Scan(&mode, &duration, &minutes); err != nil {
			return Progress{}, err
		}
		switch mode {
		case Driving:
			progress.DrivingDuration = duration
			progress.DrivingMinutes = minutes
		case Paused:
			progress.PausedDuration = duration
			progress.PausedMinutes = minutes
		}
	}
	if err = rows.Err(); err != nil {
		return Progress{}, err
	}
	progress.AmountTyiyn = estimatedAmount(rental.Tariff, progress.DrivingMinutes, progress.PausedMinutes)
	return progress, nil
}

// estimatedAmount prices the minutes begun in each mode at the rates stored with the rental rather
// than at the price list as it stands now: a change of the catalog does not reach a ride that has
// already begun.
func estimatedAmount(price tariffs.Tariff, drivingMinutes, pausedMinutes int64) int64 {
	return drivingMinutes*price.DrivingRateTyiynPerStartedMinute +
		pausedMinutes*price.PausedRateTyiynPerStartedMinute
}
