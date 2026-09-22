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

func DeleteExpired(ctx context.Context, pool *pgxpool.Pool, batchSize int) (int64, error) {
	removed, err := pool.Exec(ctx, deleteExpiredStatement, RetentionPeriod, batchSize)
	if err != nil {
		return 0, err
	}
	return removed.RowsAffected(), nil
}
