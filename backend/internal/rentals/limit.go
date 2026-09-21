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

func readDailyLimit(
	ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, at time.Time,
) (DailyLimit, error) {
	metadata, err := NewStore(pool).Metadata(ctx)
	if err != nil {
		return DailyLimit{}, err
	}
	var limit DailyLimit
	err = database.QuerierFrom(ctx, pool).QueryRow(ctx, dailyLimitStatement, userID, at, metadata.Timezone).
		Scan(&limit.Available, &limit.ResetsAt)
	return limit, err
}
