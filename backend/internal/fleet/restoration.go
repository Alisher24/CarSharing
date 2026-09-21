package fleet

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const restoreEnergySourceStatement = `
UPDATE vehicle_energy_sources
SET capacity = $3::numeric, remaining = $4::numeric
WHERE vehicle_id = $1 AND source_kind = $2`

const restoreTelemetryStatement = `
UPDATE vehicle_telemetry
SET position = ST_SetSRID(ST_MakePoint($2, $3), $4),
    confirmed_at = now() - make_interval(secs => $5)
WHERE vehicle_id = $1`

// Restore returns one vehicle and its dependent records to the declared demonstration state. The
// vehicle change is published through the same version rule as every other path.
func (s *Store) Restore(ctx context.Context, tx pgx.Tx, vehicle InstalledVehicle) (int64, error) {
	serviceRequired := false
	version, err := s.PublishChange(ctx, tx, vehicle.ID, VehicleChange{
		Connected:       &vehicle.Connected,
		Reporting:       &vehicle.Reporting,
		RouteID:         &vehicle.RouteID,
		ServiceRequired: &serviceRequired,
	})
	if err != nil {
		return 0, err
	}
	if err = restoreEnergySources(ctx, tx, vehicle); err != nil {
		return 0, err
	}
	_, err = tx.Exec(
		ctx,
		restoreTelemetryStatement,
		vehicle.ID,
		vehicle.Position.Longitude,
		vehicle.Position.Latitude,
		WGS84SRID,
		vehicle.ConfirmedAgo.Seconds(),
	)
	return version, err
}

func restoreEnergySources(ctx context.Context, tx pgx.Tx, vehicle InstalledVehicle) error {
	for _, source := range vehicle.Sources {
		if _, err := tx.Exec(
			ctx,
			restoreEnergySourceStatement,
			vehicle.ID,
			source.Kind,
			source.Capacity.Decimal(),
			source.Remaining.Decimal(),
		); err != nil {
			return err
		}
	}
	return nil
}
