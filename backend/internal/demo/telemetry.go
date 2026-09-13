package demo

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
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

// A confirmation changes when a published position was confirmed and therefore what the catalog says
// about its freshness, so it raises the version of every vehicle it confirms and reports what each
// one reached. One statement confirms the vehicles and raises their versions, so a confirmation and
// the version it publishes cannot come apart.
const confirmReportingVehicles = `
WITH confirmed AS (
    UPDATE vehicle_telemetry
    SET confirmed_at = now()
    WHERE vehicle_id IN (SELECT id FROM vehicles WHERE reporting)
    RETURNING vehicle_id
), raised AS (
    UPDATE vehicles
    SET version = version + 1
    FROM confirmed
    WHERE vehicles.id = confirmed.vehicle_id
    RETURNING vehicles.id, vehicles.version
)
SELECT id, version FROM raised`

// Confirm records one arrival from every vehicle that is still reporting and returns how many
// vehicles confirmed. The signals of the new versions are recorded with the confirmation itself: a
// reading that was stored without telling anybody would leave every map showing an older one.
func (c *Confirmations) Confirm(ctx context.Context) (int64, error) {
	var confirmed int64
	err := database.InTransaction(ctx, c.pool, func(txCtx context.Context) error {
		rows, err := database.QuerierFrom(txCtx, c.pool).Query(txCtx, confirmReportingVehicles)
		if err != nil {
			return err
		}
		signals, err := confirmationSignals(rows)
		if err != nil {
			return err
		}
		confirmed = int64(len(signals))
		return events.Record(txCtx, c.pool, signals...)
	})
	return confirmed, err
}

// confirmationSignals reads what each confirmed vehicle reached and states the public change it is.
func confirmationSignals(rows pgx.Rows) ([]events.Signal, error) {
	defer rows.Close()
	var signals []events.Signal
	for rows.Next() {
		var vehicleID string
		var version int64
		if err := rows.Scan(&vehicleID, &version); err != nil {
			return nil, err
		}
		signals = append(signals, events.Signal{
			Kind:       events.VehicleChanged,
			ResourceID: vehicleID,
			Version:    version,
		})
	}
	return signals, rows.Err()
}
