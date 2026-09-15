package fleet

import (
	"context"
	"encoding/json"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// SimulatedVehicle is one vehicle as the model moves it: the route it travels, what each source holds
// when full and what it holds now, and where it stands. It carries none of the catalog's presentation
// — no name, no version, no status — because the model moves a vehicle rather than describing one.
type SimulatedVehicle struct {
	ID             string
	PowertrainType PowertrainType

	// RouteID is the trajectory the vehicle travels, which the demonstration placed it on. A
	// vehicle without one is not moved by the model at all.
	RouteID string

	Position        Position
	Connected       bool
	Reporting       bool
	ServiceRequired bool
	Sources         []EnergySource
}

// Confirming reports whether the vehicle publishes what it does. A vehicle that is linked and
// reporting confirms its position and its reserves; one that is not keeps the reading it last
// confirmed, which then ages out of freshness on the ordinary rule.
func (v SimulatedVehicle) Confirming() bool { return v.Connected && v.Reporting }

// simulatedVehicles selects every vehicle the model travels a route along. A vehicle that has never
// reported has no position to start from and is left out, exactly as the catalog leaves it out.
const simulatedVehicles = `
SELECT
    vehicle.id,
    vehicle.powertrain_type,
    vehicle.route_id,
    vehicle.connected,
    vehicle.reporting,
    vehicle.service_required,
    ST_X(telemetry.position),
    ST_Y(telemetry.position),
    coalesce(inventory.sources, '[]'::jsonb)
FROM vehicles vehicle
JOIN vehicle_telemetry telemetry ON telemetry.vehicle_id = vehicle.id
LEFT JOIN LATERAL (` + energyInventory + `
) inventory ON true
WHERE vehicle.route_id IS NOT NULL`

// The model reads the fleet in the order the identifiers name it, so two readings of an unchanged
// fleet describe the same vehicles in the same sequence.
const (
	allSimulatedVehicles = simulatedVehicles + `
ORDER BY vehicle.id`

	oneSimulatedVehicle = simulatedVehicles + `
AND vehicle.id = $2`
)

// Simulated reads the fleet the model moves. It is a read of the fleet rather than of the model: what
// the model has made of these vehicles since is the simulation module's to store.
func (s *Store) Simulated(ctx context.Context) ([]SimulatedVehicle, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, allSimulatedVehicles, AmountScale)
	if err != nil {
		return nil, err
	}
	return collectSimulated(rows)
}

// SimulatedVehicle reads one vehicle of the fleet the model moves. A vehicle no route was placed on,
// or one that has never confirmed a position, is reported as absent rather than described by a guess
// at where it stands.
func (s *Store) SimulatedVehicle(ctx context.Context, id string) (SimulatedVehicle, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, oneSimulatedVehicle, AmountScale, id)
	if err != nil {
		return SimulatedVehicle{}, err
	}
	vehicles, err := collectSimulated(rows)
	if err != nil {
		return SimulatedVehicle{}, err
	}
	if len(vehicles) == 0 {
		return SimulatedVehicle{}, ErrVehicleNotFound
	}
	return vehicles[0], nil
}

func collectSimulated(rows pgx.Rows) ([]SimulatedVehicle, error) {
	defer rows.Close()

	vehicles := []SimulatedVehicle{}
	for rows.Next() {
		vehicle, err := scanSimulatedVehicle(rows)
		if err != nil {
			return nil, err
		}
		vehicles = append(vehicles, vehicle)
	}
	return vehicles, rows.Err()
}

func scanSimulatedVehicle(rows pgx.Rows) (SimulatedVehicle, error) {
	var vehicle SimulatedVehicle
	var inventory []byte
	err := rows.Scan(
		&vehicle.ID,
		&vehicle.PowertrainType,
		&vehicle.RouteID,
		&vehicle.Connected,
		&vehicle.Reporting,
		&vehicle.ServiceRequired,
		&vehicle.Position.Longitude,
		&vehicle.Position.Latitude,
		&inventory,
	)
	if err != nil {
		return SimulatedVehicle{}, err
	}
	var stored []storedSource
	if err = json.Unmarshal(inventory, &stored); err != nil {
		return SimulatedVehicle{}, err
	}
	vehicle.Sources = make([]EnergySource, 0, len(stored))
	for _, source := range stored {
		vehicle.Sources = append(vehicle.Sources, EnergySource(source))
	}
	return vehicle, nil
}
