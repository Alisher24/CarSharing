package simulation

import (
	"context"
	"math/big"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the modelled state of the fleet. Every statement runs on the querier the context carries,
// so a model saved inside a command's transaction is saved with that command rather than beside it.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// The modelled state of one vehicle. The part of a millionth a source has been charged but not yet
// spent is an arbitrary-precision number, so it is carried as text in both directions: a charge is
// the product of a capacity and an hour of nanoseconds, which no 64-bit column holds.
const (
	storedStatesStatement = `
SELECT vehicle_id, route_id, path, join_path, is_off_route, longitude, latitude, depleted, processed_at
FROM simulation_states
WHERE vehicle_id = ANY($1)
ORDER BY vehicle_id`

	storedSourcesStatement = `
SELECT vehicle_id, source_kind, capacity, charged::text
FROM simulation_sources
WHERE vehicle_id = ANY($1)
ORDER BY vehicle_id, source_kind`

	saveStateStatement = `
INSERT INTO simulation_states (
    vehicle_id, route_id, path, join_path, is_off_route, longitude, latitude, depleted, processed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (vehicle_id) DO UPDATE SET
    route_id = EXCLUDED.route_id,
    path = EXCLUDED.path,
    join_path = EXCLUDED.join_path,
    is_off_route = EXCLUDED.is_off_route,
    longitude = EXCLUDED.longitude,
    latitude = EXCLUDED.latitude,
    depleted = EXCLUDED.depleted,
    processed_at = EXCLUDED.processed_at`

	saveSourceStatement = `
INSERT INTO simulation_sources (vehicle_id, source_kind, capacity, charged)
VALUES ($1, $2, $3, $4::numeric)
ON CONFLICT (vehicle_id, source_kind) DO UPDATE SET
    capacity = EXCLUDED.capacity,
    charged = EXCLUDED.charged`

	dropAbsentSourcesStatement = `
DELETE FROM simulation_sources WHERE vehicle_id = $1 AND source_kind <> ALL($2)`

	forgetStatesStatement = `
DELETE FROM simulation_states WHERE vehicle_id = ANY($1)`
)

// States reads the stored model of the named vehicles. A vehicle the model has never moved has no
// entry, which is how a caller tells a fleet it must initialize from one it can carry on with.
func (s *Store) States(ctx context.Context, vehicleIDs []string) (map[string]State, error) {
	states, err := s.readStates(ctx, vehicleIDs)
	if err != nil {
		return nil, err
	}
	return s.attachSources(ctx, vehicleIDs, states)
}

// readStates reads one row per vehicle, which is what the primary key guarantees.
func (s *Store) readStates(ctx context.Context, vehicleIDs []string) (map[string]State, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, storedStatesStatement, vehicleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make(map[string]State, len(vehicleIDs))
	for rows.Next() {
		var (
			vehicleID string
			found     State
		)
		if err = rows.Scan(
			&vehicleID,
			&found.RouteID,
			&found.Path,
			&found.JoinPath,
			&found.IsOffRoute,
			&found.Position.Longitude,
			&found.Position.Latitude,
			&found.Depleted,
			&found.ProcessedAt,
		); err != nil {
			return nil, err
		}
		states[vehicleID] = found
	}
	return states, rows.Err()
}

// attachSources gives every state the inventories it moves on. The capacities of those inventories
// are the parameters of the ride, so a vehicle carries one declaration of what its sources hold
// rather than a list of reserves beside a list of capacities.
func (s *Store) attachSources(
	ctx context.Context, vehicleIDs []string, states map[string]State,
) (map[string]State, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, storedSourcesStatement, vehicleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			vehicleID string
			source    Source
			charged   string
			capacity  int64
		)
		if err = rows.Scan(&vehicleID, &source.Kind, &capacity, &charged); err != nil {
			return nil, err
		}
		found, known := states[vehicleID]
		if !known {
			continue
		}
		chargedTotal, parsed := new(big.Int).SetString(charged, 10)
		if !parsed {
			return nil, ErrStateUnusable
		}
		source.Capacity = fleet.Amount(capacity)
		source.Rate = RateOf(source.Capacity)
		source.Charged = chargedTotal
		found.Sources = append(found.Sources, source)
		states[vehicleID] = found
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for vehicleID, found := range states {
		found.Parameters = ParametersOf(capacitiesOf(found.Sources))
		states[vehicleID] = found
	}
	return states, nil
}

// capacitiesOf is what each source of a vehicle holds when it is full, which is the whole of what the
// model's parameters state.
func capacitiesOf(sources []Source) map[fleet.SourceKind]fleet.Amount {
	capacities := make(map[fleet.SourceKind]fleet.Amount, len(sources))
	for _, source := range sources {
		capacities[source.Kind] = source.Capacity
	}
	return capacities
}

// Save writes the model of one vehicle. The state and its inventories are written together, and an
// inventory the state no longer carries is removed, so what a restart reads back is one model rather
// than a model beside the remains of an earlier one.
func (s *Store) Save(ctx context.Context, vehicleID string, state State) error {
	querier := database.QuerierFrom(ctx, s.pool)
	if _, err := querier.Exec(ctx, saveStateStatement,
		vehicleID,
		string(state.RouteID),
		int64(state.Path),
		int64(state.JoinPath),
		state.IsOffRoute,
		state.Position.Longitude,
		state.Position.Latitude,
		state.Depleted,
		state.ProcessedAt,
	); err != nil {
		return err
	}
	carried := make([]string, 0, len(state.Sources))
	for _, source := range state.Sources {
		carried = append(carried, string(source.Kind))
		if _, err := querier.Exec(ctx, saveSourceStatement,
			vehicleID,
			string(source.Kind),
			int64(source.Capacity),
			source.Charged.String(),
		); err != nil {
			return err
		}
	}
	_, err := querier.Exec(ctx, dropAbsentSourcesStatement, vehicleID, carried)
	return err
}

// Forget drops the model of the named vehicles, so the next reading of them begins from what the
// fleet states they are rather than from what the model has made of them since. It is how a command
// that puts prepared vehicles back also puts their journeys back: a reserve restored to a vehicle
// whose model still holds what a previous ride spent would be a vehicle in two states at once.
func (s *Store) Forget(ctx context.Context, vehicleIDs []string) error {
	if len(vehicleIDs) == 0 {
		return nil
	}
	_, err := database.QuerierFrom(ctx, s.pool).Exec(ctx, forgetStatesStatement, vehicleIDs)
	return err
}
