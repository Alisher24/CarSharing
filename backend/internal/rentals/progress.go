package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
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

// modeDurationsStatement sums the intervals of one ride per mode. It sums and does not round: the
// billing policy is declared in the billing package and applied there, so the database holds no
// second copy of the rule. The sum reads the interval the ride is in up to the answered moment, and no
// interval that began after it, so a read taken before a transition never counts time the transition
// has not made yet.
const modeDurationsStatement = `
SELECT mode,
       COALESCE(sum(COALESCE(ended_at, $2::timestamptz) - started_at), '0'::interval)
FROM ride_segments
WHERE rental_id = $1 AND started_at <= $2
GROUP BY mode`

// readProgress builds what a ride has taken from its own intervals and the rates stored with the
// rental. A charge the billing module refuses is reported rather than published, so an amount outside
// the range the contract states cannot reach a client.
func readProgress(
	ctx context.Context, pool *pgxpool.Pool, rental Rental, moment time.Time,
) (Progress, error) {
	durations, err := readModeDurations(ctx, pool, rental.ID, moment)
	if err != nil {
		return Progress{}, err
	}
	return rental.progressFrom(durations)
}

// progressFrom prices the durations of one ride at the rates of the rental. The sum is per mode and
// rounded once by the billing module, so this function only carries its answer into the shape the
// contract publishes.
func (r Rental) progressFrom(durations modeDurations) (Progress, error) {
	charge, err := billing.Compute(ratesOf(r), durations.driving, durations.paused)
	if err != nil {
		return Progress{}, err
	}
	return Progress{
		DrivingDuration: charge.DrivingDuration,
		PausedDuration:  charge.PausedDuration,
		DrivingMinutes:  charge.DrivingMinutes,
		PausedMinutes:   charge.PausedMinutes,
		AmountTyiyn:     charge.TotalTyiyn,
	}, nil
}

// ratesOf names the rates of the price list stored with the rental, which is the price list the
// reservation was made under rather than the catalog as it stands now.
func ratesOf(rental Rental) billing.Rates {
	return billing.Rates{
		Driving: rental.Tariff.DrivingRateTyiynPerStartedMinute,
		Paused:  rental.Tariff.PausedRateTyiynPerStartedMinute,
	}
}

// modeDurations is how long one ride spent in each mode by one moment.
type modeDurations struct {
	driving time.Duration
	paused  time.Duration
}

// readModeDurations reads the sum of each mode. A mode the ride never entered has no row, and its
// duration is the zero one, which is what a ride that never paused has spent standing still.
func readModeDurations(
	ctx context.Context, pool *pgxpool.Pool, rentalID string, moment time.Time,
) (modeDurations, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, modeDurationsStatement, rentalID, moment)
	if err != nil {
		return modeDurations{}, err
	}
	defer rows.Close()

	var durations modeDurations
	for rows.Next() {
		var (
			mode     Mode
			duration time.Duration
		)
		if err = rows.Scan(&mode, &duration); err != nil {
			return modeDurations{}, err
		}
		switch mode {
		case Driving:
			durations.driving = duration
		case Paused:
			durations.paused = duration
		}
	}
	return durations, rows.Err()
}
