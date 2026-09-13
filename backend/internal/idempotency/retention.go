package idempotency

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// How long a stored result is kept and how a worker removes it. The periods are this package's own:
// a result that is removed too early is a repeat that performs the change a second time.
const (
	// RetentionInterval is how often a worker sweeps the table. The sweep is an indexed delete over
	// a set that stays small, so running it often is what keeps each portion small.
	RetentionInterval = 10 * time.Second

	// RetentionBatchSize is how many rows one sweep removes, so a table that has accumulated for a
	// long time is emptied over several sweeps instead of in one long transaction.
	RetentionBatchSize = 1000
)

// deleteExpiredStatement removes the oldest results whose retention has passed. A row is never
// removed before its retention: the sweep reads the moment from the database rather than from the
// process, so two workers cannot disagree about which rows are due.
const deleteExpiredStatement = `
DELETE FROM idempotency_requests
WHERE (user_id, command_key) IN (
    SELECT user_id, command_key
    FROM idempotency_requests
    WHERE retain_until IS NOT NULL AND retain_until <= clock_timestamp()
    ORDER BY retain_until
    LIMIT $1
)`

// Reaper removes command results once their retention has passed. It is one of the recurring jobs of
// the worker process, which is the only place that decides a stored result is old enough to drop.
type Reaper struct{ pool *pgxpool.Pool }

func NewReaper(pool *pgxpool.Pool) *Reaper { return &Reaper{pool: pool} }

// Delete runs one sweep and reports how many results it removed.
func (r *Reaper) Delete(ctx context.Context) (int64, error) {
	removed, err := database.QuerierFrom(ctx, r.pool).Exec(ctx, deleteExpiredStatement, RetentionBatchSize)
	if err != nil {
		return 0, err
	}
	return removed.RowsAffected(), nil
}
