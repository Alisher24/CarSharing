package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DailyLimit is the allowance of free reservations of one account on one calendar day of the service
// timezone. It is read whether or not a rental is current: having nothing current says nothing about
// whether the day's allowance is still there.
type DailyLimit struct {
	// Available is whether the account may still create a reservation on this day.
	Available bool

	// ResetsAt is the moment the allowance returns, being the start of the next day in the service
	// timezone.
	ResetsAt time.Time
}

// dailyLimitStatement reads the allowance of one account at the moment a transaction fixed.
//
// The day is decided from that moment rather than from the connection's own date or from any clock a
// client sent, and it is decided in the timezone the installation declares rather than in UTC: a
// reservation made at 23:55 in Bishkek spends the day that is ending there. The timezone comes from
// the same row the readiness operation publishes, so the two cannot describe the service
// differently.
//
// What spends the allowance is the reservation itself, whatever became of it afterwards: a
// cancelled, expired or completed rental still counts, which is why the statement reads the rentals
// rather than keeping a counter that could disagree with them.
const dailyLimitStatement = `
SELECT
    NOT EXISTS (
        SELECT 1
        FROM rentals rental
        WHERE rental.user_id = $1
          AND (rental.reserved_at AT TIME ZONE meta.timezone)::date
              = ($2::timestamptz AT TIME ZONE meta.timezone)::date
    ),
    (date_trunc('day', $2::timestamptz AT TIME ZONE meta.timezone) + interval '1 day')
        AT TIME ZONE meta.timezone
FROM bootstrap_metadata meta
WHERE meta.singleton`

func readDailyLimit(
	ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, at time.Time,
) (DailyLimit, error) {
	var limit DailyLimit
	err := database.QuerierFrom(ctx, pool).QueryRow(ctx, dailyLimitStatement, userID, at).
		Scan(&limit.Available, &limit.ResetsAt)
	return limit, err
}
