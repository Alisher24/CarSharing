// Package tariffs reads the prices the operator charges. A price is published as the whole number
// of tyiyn the operator set: nothing here substitutes a zero for a tariff that could not be read.
package tariffs

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tariff is one price list. Both rates are whole tyiyn per started minute of their mode, so the
// interface can show som without a rounding step of its own.
type Tariff struct {
	ID                               string
	Currency                         string
	BillingPolicy                    string
	DrivingRateTyiynPerStartedMinute int64
	PausedRateTyiynPerStartedMinute  int64
	Version                          int64
}

// Store reads the price lists currently in force.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const currentTariffs = `
SELECT
    id,
    currency,
    billing_policy,
    driving_rate_tyiyn_per_started_minute,
    paused_rate_tyiyn_per_started_minute,
    version
FROM tariffs
ORDER BY id`

// Current reads every price list in force, in a stable order.
func (s *Store) Current(ctx context.Context) ([]Tariff, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, currentTariffs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := []Tariff{}
	for rows.Next() {
		var tariff Tariff
		err = rows.Scan(
			&tariff.ID,
			&tariff.Currency,
			&tariff.BillingPolicy,
			&tariff.DrivingRateTyiynPerStartedMinute,
			&tariff.PausedRateTyiynPerStartedMinute,
			&tariff.Version,
		)
		if err != nil {
			return nil, err
		}
		found = append(found, tariff)
	}
	return found, rows.Err()
}
