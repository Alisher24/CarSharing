package fleet

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// InstalledVehicle is the fleet-owned shape used to install or restore one vehicle and its dependent
// records. The operation receives values, not table details, so the demonstration does not learn the
// storage schema it populates.
type InstalledVehicle struct {
	ID             string
	Model          string
	PowertrainType PowertrainType
	Connected      bool
	Reporting      bool
	RouteID        string
	Position       Position
	Sources        []EnergySource
	ConfirmedAgo   time.Duration
}

const insertVehicleStatement = `
INSERT INTO vehicles (id, model, powertrain_type, connected, reporting, route_id, version)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO NOTHING`

const installedRouteSelection = `SELECT route_id FROM vehicles WHERE id = $1`

const insertEnergySourceStatement = `
INSERT INTO vehicle_energy_sources (vehicle_id, source_kind, capacity, remaining)
VALUES ($1, $2, $3::numeric, $4::numeric)
ON CONFLICT (vehicle_id, source_kind) DO NOTHING`

const insertTelemetryStatement = `
INSERT INTO vehicle_telemetry (vehicle_id, position, confirmed_at)
VALUES ($1, ST_SetSRID(ST_MakePoint($2, $3), $4), now() - make_interval(secs => $5))
ON CONFLICT (vehicle_id) DO NOTHING`

// Install creates the fleet records that are absent and leaves existing ones unchanged, except for a
// route introduced by a newer declaration.
func (s *Store) Install(ctx context.Context, tx pgx.Tx, vehicle InstalledVehicle) error {
	inserted, err := tx.Exec(
		ctx,
		insertVehicleStatement,
		vehicle.ID,
		vehicle.Model,
		vehicle.PowertrainType,
		vehicle.Connected,
		vehicle.Reporting,
		vehicle.RouteID,
		database.InitialVersion,
	)
	if err != nil {
		return err
	}
	if inserted.RowsAffected() == 0 {
		if err = s.installRoute(ctx, tx, vehicle.ID, vehicle.RouteID); err != nil {
			return err
		}
	}
	if err = installEnergySources(ctx, tx, vehicle); err != nil {
		return err
	}
	return installTelemetry(ctx, tx, vehicle)
}

func (s *Store) installRoute(ctx context.Context, tx pgx.Tx, vehicleID, routeID string) error {
	var installed *string
	if err := tx.QueryRow(ctx, installedRouteSelection, vehicleID).Scan(&installed); err != nil {
		return err
	}
	if installed != nil && *installed == routeID {
		return nil
	}
	_, err := s.PublishChange(ctx, tx, vehicleID, VehicleChange{RouteID: &routeID})
	return err
}

func installEnergySources(ctx context.Context, tx pgx.Tx, vehicle InstalledVehicle) error {
	for _, source := range vehicle.Sources {
		if _, err := tx.Exec(
			ctx,
			insertEnergySourceStatement,
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

func installTelemetry(ctx context.Context, tx pgx.Tx, vehicle InstalledVehicle) error {
	_, err := tx.Exec(
		ctx,
		insertTelemetryStatement,
		vehicle.ID,
		vehicle.Position.Longitude,
		vehicle.Position.Latitude,
		WGS84SRID,
		vehicle.ConfirmedAgo.Seconds(),
	)
	return err
}
