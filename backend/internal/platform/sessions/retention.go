package sessions

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// How expired sessions are kept from accumulating. A row is created by every sign-in and removed by
// nothing else but a sign-out, so a table nothing sweeps grows with every session that was ever
// established and never explicitly ended.
const (
	// RetentionBatchSize is how many rows one sweep removes, so a table that has accumulated for a
	// long time is emptied over several sweeps instead of in one long transaction.
	RetentionBatchSize = 1000

	// RetentionInterval is how often a worker sweeps the table.
	RetentionInterval = 10 * time.Second
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

// Reaper removes sessions whose expiry has passed. It is one of the recurring jobs of the worker
// process, which is the only place that decides a session is old enough to drop.
type Reaper struct{ pool *pgxpool.Pool }

func NewReaper(pool *pgxpool.Pool) *Reaper { return &Reaper{pool: pool} }

// Delete runs one sweep and reports how many sessions it removed.
func (r *Reaper) Delete(ctx context.Context) (int64, error) {
	removed, err := database.QuerierFrom(ctx, r.pool).Exec(ctx, deleteExpiredStatement, RetentionBatchSize)
	if err != nil {
		return 0, err
	}
	return removed.RowsAffected(), nil
}
