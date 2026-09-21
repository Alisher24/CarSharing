package tariffs

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const insertTariffStatement = `
INSERT INTO tariffs (
    id,
    currency,
    billing_policy,
    driving_rate_tyiyn_per_started_minute,
    paused_rate_tyiyn_per_started_minute,
    version
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO NOTHING`

// Install creates a price list that is absent and leaves an existing one unchanged.
func (s *Store) Install(ctx context.Context, tx pgx.Tx, tariff Tariff) error {
	_, err := tx.Exec(
		ctx,
		insertTariffStatement,
		tariff.ID,
		tariff.Currency,
		tariff.BillingPolicy,
		tariff.DrivingRateTyiynPerStartedMinute,
		tariff.PausedRateTyiynPerStartedMinute,
		tariff.Version,
	)
	return err
}
