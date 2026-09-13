package events

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// serverTimeStatement reads the database clock, which is the authoritative time of every response
// this application states.
const serverTimeStatement = `SELECT now()`

func serverTime(ctx context.Context, pool *pgxpool.Pool) (time.Time, error) {
	var now time.Time
	err := database.QuerierFrom(ctx, pool).QueryRow(ctx, serverTimeStatement).Scan(&now)
	return now, err
}
