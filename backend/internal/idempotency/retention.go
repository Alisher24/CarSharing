package idempotency

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
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

func DeleteExpired(ctx context.Context, pool *pgxpool.Pool, batchSize int) (int64, error) {
	removed, err := database.QuerierFrom(ctx, pool).Exec(ctx, deleteExpiredStatement, batchSize)
	if err != nil {
		return 0, err
	}
	return removed.RowsAffected(), nil
}
