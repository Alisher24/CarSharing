package demo

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConfirmationInterval is how often the demonstration source confirms where the fleet is standing.
// It is comfortably inside fleet.MaxTelemetryAge, so an ordinary vehicle stays fresh between two
// confirmations and a vehicle the source skips goes stale on the ordinary rule instead.
const ConfirmationInterval = fleet.MaxTelemetryAge / 3

// Confirmations is the demonstration source of telemetry. It stands in for the vehicles of a
// demonstration that do not exist outside it, and it publishes what the model of each vehicle holds:
// where it stands and what is left in its sources. A vehicle the source does not confirm keeps the
// reading it last confirmed, which is what lets one demonstration show a position that has aged out
// of freshness rather than a position that was quietly replaced by a prediction.
//
// Reading the catalog is not a confirmation. Only an arrival recorded here makes a position fresh.
type Confirmations struct {
	pool   *pgxpool.Pool
	models *simulation.Store
}

func NewConfirmations(pool *pgxpool.Pool, models *simulation.Store) *Confirmations {
	return &Confirmations{pool: pool, models: models}
}

// reportingVehiclesStatement names the vehicles that are sending telemetry. A vehicle that is not
// linked and one that is linked but silent are both left out, which is the difference this source
// exists to demonstrate.
const reportingVehiclesStatement = `
SELECT id FROM vehicles WHERE reporting ORDER BY id`

// confirmPositionStatement publishes where the named vehicles stand, at one moment read from the
// database: a confirmation that dated each vehicle by its own clock would let two vehicles in one
// answer claim to have been confirmed at different instants.
const confirmPositionStatement = `
UPDATE vehicle_telemetry telemetry
SET position = ST_SetSRID(ST_MakePoint(confirmed.longitude, confirmed.latitude), $4),
    confirmed_at = clock_timestamp()
FROM unnest($1::uuid[], $2::double precision[], $3::double precision[])
    AS confirmed(vehicle_id, longitude, latitude)
WHERE telemetry.vehicle_id = confirmed.vehicle_id`

// confirmReserveStatement publishes what each source of the named vehicles holds now.
const confirmReserveStatement = `
UPDATE vehicle_energy_sources source
SET remaining = confirmed.remaining
FROM unnest($1::uuid[], $2::text[], $3::numeric[])
    AS confirmed(vehicle_id, source_kind, remaining)
WHERE source.vehicle_id = confirmed.vehicle_id
  AND source.source_kind = confirmed.source_kind`

// refreshTelemetryStatement moves the moment of a confirmation for vehicles nothing is modelled for:
// a vehicle the model does not travel still reports that it is where it was.
const refreshTelemetryStatement = `
UPDATE vehicle_telemetry
SET confirmed_at = clock_timestamp()
WHERE vehicle_id = ANY($1)`

// raisedVersionsStatement raises the version of every confirmed vehicle in one statement, so a
// confirmation and the change it publishes cannot come apart.
const raisedVersionsStatement = `
UPDATE vehicles
SET version = version + 1
WHERE id = ANY($1)
RETURNING id, version`

// Confirm records one arrival from every vehicle that is still reporting and returns how many
// vehicles confirmed. The signals of the new versions are recorded with the confirmation itself: a
// reading that was stored without telling anybody would leave every map showing an older one.
func (c *Confirmations) Confirm(ctx context.Context) (int64, error) {
	var confirmed int64
	err := database.InTransaction(ctx, c.pool, func(txCtx context.Context) error {
		reporting, err := c.reportingVehicles(txCtx)
		if err != nil {
			return err
		}
		if len(reporting) == 0 {
			return nil
		}
		if err = c.publishArrivals(txCtx, reporting); err != nil {
			return err
		}
		signals, err := confirmationSignals(txCtx, c.pool, reporting)
		if err != nil {
			return err
		}
		confirmed = int64(len(signals))
		return events.Record(txCtx, c.pool, signals...)
	})
	return confirmed, err
}

func (c *Confirmations) reportingVehicles(ctx context.Context) ([]string, error) {
	rows, err := database.QuerierFrom(ctx, c.pool).Query(ctx, reportingVehiclesStatement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reporting := []string{}
	for rows.Next() {
		var vehicleID string
		if err = rows.Scan(&vehicleID); err != nil {
			return nil, err
		}
		reporting = append(reporting, vehicleID)
	}
	return reporting, rows.Err()
}

// confirmedArrivals is one confirmation of the whole reporting fleet, stated as the arrays the two
// statements read: where each modelled vehicle stands, and what each of its sources holds. The three
// reserve arrays are read by one index, so they are built together rather than joined afterwards.
type confirmedArrivals struct {
	vehicleIDs []string
	longitudes []float64
	latitudes  []float64

	reserveVehicleIDs []string
	reserveKinds      []string
	reserveAmounts    []string

	unmodelled []string
}

// publishArrivals writes the confirmed reading of every reporting vehicle: where the model stands and
// what it holds for the vehicles it travels, and the moment alone for the vehicles it does not.
func (c *Confirmations) publishArrivals(ctx context.Context, reporting []string) error {
	states, err := c.models.States(ctx, reporting)
	if err != nil {
		return err
	}
	arrivals := arrivalsOf(reporting, states)
	querier := database.QuerierFrom(ctx, c.pool)
	if len(arrivals.vehicleIDs) > 0 {
		if _, err = querier.Exec(ctx, confirmPositionStatement, arrivals.vehicleIDs,
			arrivals.longitudes, arrivals.latitudes, wgs84SRID); err != nil {
			return err
		}
	}
	if len(arrivals.reserveKinds) > 0 {
		if _, err = querier.Exec(ctx, confirmReserveStatement, arrivals.reserveVehicleIDs,
			arrivals.reserveKinds, arrivals.reserveAmounts); err != nil {
			return err
		}
	}
	if len(arrivals.unmodelled) == 0 {
		return nil
	}
	_, err = querier.Exec(ctx, refreshTelemetryStatement, arrivals.unmodelled)
	return err
}

// arrivalsOf states what one confirmation publishes about the reporting fleet.
func arrivalsOf(reporting []string, states map[string]simulation.State) confirmedArrivals {
	var arrivals confirmedArrivals
	for _, vehicleID := range reporting {
		state, modelled := states[vehicleID]
		if !modelled {
			// A vehicle nothing is modelled for still reports that it is where it was: the moment of
			// its reading moves even though its position and its reserves do not.
			arrivals.unmodelled = append(arrivals.unmodelled, vehicleID)
			continue
		}
		arrivals.vehicleIDs = append(arrivals.vehicleIDs, vehicleID)
		arrivals.longitudes = append(arrivals.longitudes, state.Position.Longitude)
		arrivals.latitudes = append(arrivals.latitudes, state.Position.Latitude)
		for _, source := range state.Sources {
			arrivals.reserveVehicleIDs = append(arrivals.reserveVehicleIDs, vehicleID)
			arrivals.reserveKinds = append(arrivals.reserveKinds, string(source.Kind))
			arrivals.reserveAmounts = append(arrivals.reserveAmounts, source.Remaining().Decimal())
		}
	}
	return arrivals
}

// confirmationSignals raises the version of every confirmed vehicle and states the public change each
// one is.
func confirmationSignals(
	ctx context.Context, pool *pgxpool.Pool, reporting []string,
) ([]events.Signal, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, raisedVersionsStatement, reporting)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var signals []events.Signal
	for rows.Next() {
		var (
			vehicleID string
			version   int64
		)
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
