package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpirySweepInterval is how often reservations are checked against their deadlines. A reservation
// is therefore released within a second of running out, which is close enough for a person
// watching the map to see the vehicle free itself.
const ExpirySweepInterval = time.Second

// Expiry ends reservations whose deadline has passed. It is the rentals module's own transition:
// the catalog reads the result rather than depicting a release the database has not made.
type Expiry struct{ pool *pgxpool.Pool }

func NewExpiry(pool *pgxpool.Pool) *Expiry { return &Expiry{pool: pool} }

// expireDueReservations ends every reservation already past its deadline. The rental ends at the
// deadline itself rather than at the moment this ran, so a late sweep does not extend a
// reservation that had already run out. One statement performs the whole transition, so two sweeps
// arriving together cannot expire one reservation twice.
const expireDueReservations = `
UPDATE rentals
SET stage = $1, ended_at = expires_at
WHERE stage = $2 AND expires_at <= now()`

// ExpireDue ends every reservation that has run out and reports how many it ended.
func (e *Expiry) ExpireDue(ctx context.Context) (int64, error) {
	tag, err := database.QuerierFrom(ctx, e.pool).Exec(ctx, expireDueReservations, Expired, Reserved)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
