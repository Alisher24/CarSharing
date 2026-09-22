package sessions

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// deleteExpiredStatement removes the sessions whose expiry has passed. The moment is read from the
// database rather than from the process, so two workers cannot disagree about which rows are due. A
// session at or before the moment is already refused by every read, so removing it changes no answer.
const deleteExpiredStatement = `
DELETE FROM sessions
WHERE token IN (
    SELECT token
    FROM sessions
    WHERE expiry <= now()
    ORDER BY expiry
    LIMIT $1
)`

func DeleteExpired(ctx context.Context, pool *pgxpool.Pool, batchSize int) (int64, error) {
	removed, err := database.QuerierFrom(ctx, pool).Exec(ctx, deleteExpiredStatement, batchSize)
	if err != nil {
		return 0, err
	}
	return removed.RowsAffected(), nil
}
