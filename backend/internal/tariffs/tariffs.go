// Package tariffs reads the prices the operator charges. A price is published as the whole number
// of tyiyn the operator set: nothing here substitutes a zero for a tariff that could not be read.
package tariffs

import (
	"context"
	"errors"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
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

// ErrNoTariffInForce reports an installation that charges nothing because it holds no price list. A
// reservation cannot be made under a price list that does not exist, so the caller reports it rather
// than substituting a price of its own.
var ErrNoTariffInForce = errors.New("no price list is in force")

// inForce reads the price list a reservation is made under. The installation holds the prices the
// operator set, and the catalogue publishes them in one stable order, so the price list a command
// charges by is the first of that order rather than a second selection rule of its own.
const inForce = currentTariffs + `
LIMIT 1`

// scanTariff reads one row of the price list columns both selections state, in the order they state
// them.
func scanTariff(row pgx.Row, found *Tariff) error {
	return row.Scan(
		&found.ID,
		&found.Currency,
		&found.BillingPolicy,
		&found.DrivingRateTyiynPerStartedMinute,
		&found.PausedRateTyiynPerStartedMinute,
		&found.Version,
	)
}

// InForce reads the price list currently charged. It reads at most one row, so that an installation
// holding no price list is reported as such rather than answered with a price nobody set.
func (s *Store) InForce(ctx context.Context) (Tariff, error) {
	return database.ReadOne(ctx, database.QuerierFrom(ctx, s.pool),
		scanTariff, ErrNoTariffInForce, inForce)
}

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
		if err = scanTariff(rows, &tariff); err != nil {
			return nil, err
		}
		found = append(found, tariff)
	}
	return found, rows.Err()
}
