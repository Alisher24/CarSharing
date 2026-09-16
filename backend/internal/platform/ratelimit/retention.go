package ratelimit

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// How the counters are kept from growing without a bound. A counter row is created by every subject
// an attacker names, so a table nothing removes from grows with the attempts rather than with the
// accounts.
const (
	// RetentionPeriod is how long a counter row is kept after its window began. Any attempt within
	// this period is counted by the row that already exists; the period is a retention question
	// rather than a correctness one, because a window that has passed is replaced by the next
	// attempt whatever the row holds.
	RetentionPeriod = time.Hour

	// RetentionBatchSize is how many rows one sweep removes, so a table that has accumulated for a
	// long time is emptied over several sweeps instead of in one long transaction.
	RetentionBatchSize = 1000

	// RetentionInterval is how often a worker sweeps the table.
	RetentionInterval = 10 * time.Second
)

// deleteExpiredStatement removes the oldest counters whose window began before the retention period.
// The moment is read from the database rather than from the process, so two workers cannot disagree
// about which rows are due.
const deleteExpiredStatement = `
DELETE FROM rate_limit_counters
WHERE (scope, subject) IN (
    SELECT scope, subject
    FROM rate_limit_counters
    WHERE window_started_at + $1::interval <= now()
    ORDER BY window_started_at
    LIMIT $2
)`

// Reaper removes counters that no attempt has returned to within the retention period. It is one of
// the recurring jobs of the worker process, which is the only place that decides a counter is old
// enough to drop.
type Reaper struct{ pool *pgxpool.Pool }

func NewReaper(pool *pgxpool.Pool) *Reaper { return &Reaper{pool: pool} }

// Delete runs one sweep and reports how many counters it removed.
func (r *Reaper) Delete(ctx context.Context) (int64, error) {
	removed, err := r.pool.Exec(ctx, deleteExpiredStatement, RetentionPeriod, RetentionBatchSize)
	if err != nil {
		return 0, err
	}
	return removed.RowsAffected(), nil
}
