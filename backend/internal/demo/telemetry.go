package demo

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConfirmationInterval is how often the demonstration source confirms where the fleet is standing.
// It is comfortably inside fleet.MaxTelemetryAge, so an ordinary vehicle stays fresh between two
// confirmations and a vehicle the source skips goes stale on the ordinary rule instead.
const ConfirmationInterval = fleet.MaxTelemetryAge / 3

// Confirmations is the demonstration source of telemetry. The vehicles it stands in for do not
// move yet, so it confirms the position they are already at: without it every reading would age
// past the freshness limit within seconds and the map would have nothing fresh to show.
//
// Reading the catalog is not a confirmation. Only an arrival recorded here makes a position fresh,
// which is what lets a vehicle the source skips demonstrate genuinely stale telemetry.
type Confirmations struct{ pool *pgxpool.Pool }

func NewConfirmations(pool *pgxpool.Pool) *Confirmations {
	return &Confirmations{pool: pool}
}

// confirmReportingVehicles records a new arrival from every vehicle that is still reporting. The
// vehicles stand still, so the reading is the position they are already at; what changes is that it
// has just been confirmed.
const confirmReportingVehicles = `
UPDATE vehicle_telemetry
SET confirmed_at = now()
WHERE vehicle_id IN (SELECT id FROM vehicles WHERE reporting)`

// Confirm records one arrival from every vehicle that is still reporting and returns how many
// vehicles confirmed.
func (c *Confirmations) Confirm(ctx context.Context) (int64, error) {
	tag, err := database.QuerierFrom(ctx, c.pool).Exec(ctx, confirmReportingVehicles)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
