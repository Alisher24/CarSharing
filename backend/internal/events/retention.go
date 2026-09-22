package events

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

// How long a delivered signal task is kept. The policy covers the signals of this package only: a
// task of any other kind, and a task that has not been delivered, is not deleted by it.
const (
	// RetentionPeriod is how long a delivered signal task stays in the queue after it was delivered.
	// It is long enough to answer what was announced recently and short enough that the queue does
	// not grow with the history of a running demonstration.
	RetentionPeriod = 24 * time.Hour
)

func DeleteExpired(ctx context.Context, pool *pgxpool.Pool, batchSize int) (int64, error) {
	return outbox.NewStore(pool).DeleteCompleted(ctx, AnnouncedKindNames(), RetentionPeriod, batchSize)
}
