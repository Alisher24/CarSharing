package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrVehicleNotFound reports an identifier no published vehicle carries. A vehicle that has never
// confirmed a position is not published either, because the catalog states where a vehicle is.
var ErrVehicleNotFound = errors.New("no such vehicle")

// Snapshot is the whole published fleet as it stood at one instant. Freshness is measured against
// that instant rather than against each reader's own clock.
type Snapshot struct {
	ObservedAt time.Time
	Vehicles   []Vehicle
}

// Store reads the published fleet. It only reads: a status the catalog invented would be a second
// owner of a rental's state.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// publishedVehicles selects one row per published vehicle. The energy inventory arrives as JSON so
// that a vehicle and its sources are read in one statement, scaled to whole millionths because that
// is how an amount is counted.
const publishedVehicles = `
SELECT
    vehicle.id,
    vehicle.model,
    vehicle.powertrain_type,
    vehicle.connected,
    vehicle.version,
    telemetry.confirmed_at,
    ST_X(telemetry.position),
    ST_Y(telemetry.position),
    EXISTS (SELECT 1 FROM service_zones zone WHERE ST_Covers(zone.area, telemetry.position)),
    coalesce(live_rental.stage, ''),
    coalesce(inventory.sources, '[]'::jsonb)
FROM vehicles vehicle
JOIN vehicle_telemetry telemetry ON telemetry.vehicle_id = vehicle.id
LEFT JOIN rentals live_rental
    ON live_rental.vehicle_id = vehicle.id AND live_rental.ended_at IS NULL
LEFT JOIN LATERAL (
    SELECT jsonb_agg(
        jsonb_build_object(
            'kind', source.source_kind,
            'remaining', (source.remaining * $1)::bigint,
            'capacity', (source.capacity * $1)::bigint
        )
        ORDER BY source.source_kind
    ) AS sources
    FROM vehicle_energy_sources source
    WHERE source.vehicle_id = vehicle.id
) inventory ON true`

// The catalog is ordered by identifier, which is drawn in creation order, so two readings of an
// unchanged fleet list it the same way.
const (
	allPublishedVehicles = publishedVehicles + `
ORDER BY vehicle.id`

	onePublishedVehicle = publishedVehicles + `
WHERE vehicle.id = $2`

	observedAtQuery = `SELECT now()`
)

// Snapshot reads every published vehicle together with the instant it was read at.
func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	return s.read(ctx, allPublishedVehicles)
}

// Vehicle reads one published vehicle together with the instant it was read at.
func (s *Store) Vehicle(ctx context.Context, id string) (Snapshot, error) {
	found, err := s.read(ctx, onePublishedVehicle, id)
	if err != nil {
		return Snapshot{}, err
	}
	if len(found.Vehicles) == 0 {
		return Snapshot{}, ErrVehicleNotFound
	}
	return found, nil
}

// read answers one selection of published vehicles against one reading of the server clock, so
// every vehicle in one answer is judged fresh or stale against the same instant.
//
// The clock is read after the rows rather than before them. A confirmation landing between the two
// reads would otherwise give the answer a vehicle whose position was confirmed later than the
// answer itself claims to have been taken, which is a state no reader should have to make sense of.
func (s *Store) read(ctx context.Context, selection string, arguments ...any) (Snapshot, error) {
	querier := database.QuerierFrom(ctx, s.pool)
	rows, err := querier.Query(ctx, selection, append([]any{AmountScale}, arguments...)...)
	if err != nil {
		return Snapshot{}, err
	}
	vehicles, err := collectVehicles(rows)
	if err != nil {
		return Snapshot{}, err
	}
	var observedAt time.Time
	if err = querier.QueryRow(ctx, observedAtQuery).Scan(&observedAt); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{ObservedAt: observedAt, Vehicles: vehicles}, nil
}

// storedSource is one row of the JSON inventory the query aggregates.
type storedSource struct {
	Kind      SourceKind `json:"kind"`
	Remaining Amount     `json:"remaining"`
	Capacity  Amount     `json:"capacity"`
}

func collectVehicles(rows pgx.Rows) ([]Vehicle, error) {
	defer rows.Close()
	vehicles := []Vehicle{}
	for rows.Next() {
		vehicle, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		vehicles = append(vehicles, vehicle)
	}
	return vehicles, rows.Err()
}

func scanVehicle(rows pgx.Rows) (Vehicle, error) {
	var vehicle Vehicle
	var inventory []byte
	err := rows.Scan(
		&vehicle.ID,
		&vehicle.Model,
		&vehicle.PowertrainType,
		&vehicle.Connected,
		&vehicle.Version,
		&vehicle.Telemetry.ConfirmedAt,
		&vehicle.Telemetry.Position.Longitude,
		&vehicle.Telemetry.Position.Latitude,
		&vehicle.InsideServiceZone,
		&vehicle.HeldBy,
		&inventory,
	)
	if err != nil {
		return Vehicle{}, err
	}
	var stored []storedSource
	if err = json.Unmarshal(inventory, &stored); err != nil {
		return Vehicle{}, err
	}
	vehicle.Sources = make([]EnergySource, 0, len(stored))
	for _, source := range stored {
		vehicle.Sources = append(vehicle.Sources, EnergySource(source))
	}
	return vehicle, nil
}
