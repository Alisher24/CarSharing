package fleet

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Confirmation is one vehicle's confirmed position and source reserves.
type Confirmation struct {
	VehicleID string
	Position  Position
	Sources   []EnergySource
}

const reportingVehiclesStatement = `
SELECT id FROM vehicles WHERE reporting ORDER BY id`

// ReportingVehicleIDs returns the vehicles currently sending telemetry.
func (s *Store) ReportingVehicleIDs(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, reportingVehiclesStatement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	identifiers := []string{}
	for rows.Next() {
		var identifier string
		if err = rows.Scan(&identifier); err != nil {
			return nil, err
		}
		identifiers = append(identifiers, identifier)
	}
	return identifiers, rows.Err()
}

const confirmPositionStatement = `
UPDATE vehicle_telemetry
SET position = ST_SetSRID(ST_MakePoint($2, $3), $4),
    confirmed_at = clock_timestamp()
WHERE vehicle_id = $1`

const confirmSourceStatement = `
UPDATE vehicle_energy_sources
SET remaining = $3::numeric
WHERE vehicle_id = $1 AND source_kind = $2`

// Confirm publishes one vehicle's telemetry in the surrounding transaction.
func (s *Store) Confirm(ctx context.Context, tx pgx.Tx, confirmation Confirmation) error {
	if _, err := tx.Exec(
		ctx,
		confirmPositionStatement,
		confirmation.VehicleID,
		confirmation.Position.Longitude,
		confirmation.Position.Latitude,
		WGS84SRID,
	); err != nil {
		return err
	}
	for _, source := range confirmation.Sources {
		if _, err := tx.Exec(
			ctx,
			confirmSourceStatement,
			confirmation.VehicleID,
			source.Kind,
			source.Remaining.Decimal(),
		); err != nil {
			return err
		}
	}
	return nil
}

const refreshTelemetryStatement = `
UPDATE vehicle_telemetry
SET confirmed_at = clock_timestamp()
WHERE vehicle_id = ANY($1)`

// RefreshTelemetry confirms the existing positions of vehicles the model does not move.
func (s *Store) RefreshTelemetry(ctx context.Context, tx pgx.Tx, vehicleIDs []string) error {
	if len(vehicleIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, refreshTelemetryStatement, vehicleIDs)
	return err
}
