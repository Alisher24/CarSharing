package demo

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/Alisher24/CarSharing/backend/internal/zones"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// store writes the demonstration rows. Every statement runs on the querier the context carries, so
// a whole installation or a whole restoration commits at once or not at all.
type store struct{ pool *pgxpool.Pool }

func (s store) querier(ctx context.Context) database.Querier {
	return database.QuerierFrom(ctx, s.pool)
}

const insertZoneStatement = `
INSERT INTO service_zones (id, name, area, version)
VALUES ($1, $2, ST_SetSRID(ST_GeomFromGeoJSON($3), $4), $5)
ON CONFLICT (id) DO NOTHING`

func (s store) insertZone(ctx context.Context, zone zones.Zone) error {
	_, err := s.querier(ctx).Exec(ctx, insertZoneStatement,
		zone.ID, zone.Name, string(zone.Area), fleet.WGS84SRID, zone.Version)
	return err
}

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

func (s store) insertTariff(ctx context.Context, tariff tariffs.Tariff) error {
	_, err := s.querier(ctx).Exec(ctx, insertTariffStatement,
		tariff.ID,
		tariff.Currency,
		tariff.BillingPolicy,
		tariff.DrivingRateTyiynPerStartedMinute,
		tariff.PausedRateTyiynPerStartedMinute,
		tariff.Version,
	)
	return err
}

// vehicleInitialVersion is the public representation version a demonstration vehicle starts at.
const vehicleInitialVersion = 1

// The route is written with the vehicle rather than only when it is created: a vehicle installed by
// an earlier build carries none, and the model moves only the vehicles it can name a route for. The
// update happens only where the stored route is not the declared one, so a repeated seed still
// changes nothing and a vehicle the demonstration left for a person to book is untouched by it.
const insertVehicleStatement = `
INSERT INTO vehicles (id, model, powertrain_type, connected, reporting, route_id, version)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET route_id = EXCLUDED.route_id
WHERE vehicles.route_id IS DISTINCT FROM EXCLUDED.route_id`

func (s store) insertVehicle(ctx context.Context, vehicle Vehicle) error {
	_, err := s.querier(ctx).Exec(ctx, insertVehicleStatement,
		vehicle.ID,
		vehicle.Model,
		vehicle.PowertrainType,
		vehicle.Connected,
		vehicle.Reporting,
		string(vehicle.RouteID),
		vehicleInitialVersion,
	)
	return err
}

// An amount is written as its canonical decimal rather than as a fraction, because dividing a
// scaled integer on the way in is where an exact reserve would stop being exact.
const insertEnergySourceStatement = `
INSERT INTO vehicle_energy_sources (vehicle_id, source_kind, capacity, remaining)
VALUES ($1, $2, $3::numeric, $4::numeric)
ON CONFLICT (vehicle_id, source_kind) DO NOTHING`

func (s store) insertEnergySources(ctx context.Context, vehicle Vehicle) error {
	for _, source := range vehicle.Sources {
		_, err := s.querier(ctx).Exec(ctx, insertEnergySourceStatement,
			vehicle.ID, source.Kind, source.Capacity.Decimal(), source.Remaining.Decimal())
		if err != nil {
			return err
		}
	}
	return nil
}

// A vehicle that is not reporting is written with a reading already older than the freshness
// limit. Writing it as just confirmed would make the demonstration show a fresh position for a
// vehicle nothing is going to confirm again, and the stale example would only appear later.
const insertTelemetryStatement = `
INSERT INTO vehicle_telemetry (vehicle_id, position, confirmed_at)
VALUES ($1, ST_SetSRID(ST_MakePoint($2, $3), $4), now() - make_interval(secs => $5))
ON CONFLICT (vehicle_id) DO NOTHING`

func (s store) insertTelemetry(ctx context.Context, vehicle Vehicle) error {
	_, err := s.querier(ctx).Exec(ctx, insertTelemetryStatement,
		vehicle.ID, vehicle.Position.Longitude, vehicle.Position.Latitude, fleet.WGS84SRID,
		vehicle.confirmedAgo().Seconds())
	return err
}

// A prepared rental is inserted with the ordinary reservation deadline, measured from the database
// clock, and with the conditions it is made under taken from the price list it names. Any unique
// violation leaves it out, so a rental a person has since started on the same vehicle is never
// displaced by one that was merely prepared.
//
// A prepared ride records no interval, so the mode it is in began when the ride did: that is the one
// moment the row itself accounts for, and it is what the constraint on a started ride requires.
const insertRentalStatement = `
INSERT INTO rentals (
    id, user_id, vehicle_id, stage, tariff_id, zone_id, reserved_at, expires_at, started_at,
    mode_started_at,
    tariff_currency, tariff_billing_policy, tariff_driving_rate_tyiyn_per_started_minute,
    tariff_paused_rate_tyiyn_per_started_minute, tariff_version
)
SELECT $1, $2, $3, $4, $5, $6, now(), now() + make_interval(secs => $7),
       CASE WHEN $8 THEN now() END,
       CASE WHEN $8 THEN now() END,
       price.currency, price.billing_policy, price.driving_rate_tyiyn_per_started_minute,
       price.paused_rate_tyiyn_per_started_minute, price.version
FROM tariffs price
WHERE price.id = $5
ON CONFLICT DO NOTHING`

// preparedRental is one rental of the scenario, resolved to the row it is written from.
type preparedRental struct {
	id        string
	userID    uuid.UUID
	vehicleID string
	stage     stage.Stage
	tariffID  string
	zoneID    string
}

// preparedRental describes the rental this vehicle is set up to hold. Installing the scenario and
// restoring it both ask the vehicle, so neither can describe it differently from the other.
func (v Vehicle) preparedRental(owner uuid.UUID, tariffID, zoneID string) preparedRental {
	return preparedRental{
		id:        v.PreparedRentalID,
		userID:    owner,
		vehicleID: v.ID,
		stage:     v.HeldBy,
		tariffID:  tariffID,
		zoneID:    zoneID,
	}
}

func (s store) insertRental(ctx context.Context, prepared preparedRental) error {
	_, err := s.querier(ctx).Exec(ctx, insertRentalStatement,
		prepared.id,
		prepared.userID,
		prepared.vehicleID,
		prepared.stage,
		prepared.tariffID,
		prepared.zoneID,
		rentals.ReservationLifetime.Seconds(),
		prepared.stage != stage.Reserved,
	)
	return err
}

// A restoration puts a prepared rental back rather than creating it again: the row keeps its
// identity and its version is raised, because a version that started over would look to every client
// like a change older than the one it already shows, and would be discarded.
const restoreRentalStatement = `
INSERT INTO rentals (
    id, user_id, vehicle_id, stage, tariff_id, zone_id, reserved_at, expires_at, started_at,
    mode_started_at, version,
    tariff_currency, tariff_billing_policy, tariff_driving_rate_tyiyn_per_started_minute,
    tariff_paused_rate_tyiyn_per_started_minute, tariff_version
)
SELECT $1, $2, $3, $4, $5, $6, now(), now() + make_interval(secs => $7),
       CASE WHEN $8 THEN now() END,
       CASE WHEN $8 THEN now() END,
       $9,
       price.currency, price.billing_policy, price.driving_rate_tyiyn_per_started_minute,
       price.paused_rate_tyiyn_per_started_minute, price.version
FROM tariffs price
WHERE price.id = $5
ON CONFLICT (id) DO UPDATE SET
    stage = EXCLUDED.stage,
    tariff_id = EXCLUDED.tariff_id,
    zone_id = EXCLUDED.zone_id,
    reserved_at = EXCLUDED.reserved_at,
    expires_at = EXCLUDED.expires_at,
    started_at = EXCLUDED.started_at,
    mode_started_at = EXCLUDED.mode_started_at,
    ended_at = CASE WHEN EXCLUDED.stage IN ('reserved', 'active', 'paused') THEN NULL ELSE now() END,
    tariff_currency = EXCLUDED.tariff_currency,
    tariff_billing_policy = EXCLUDED.tariff_billing_policy,
    tariff_driving_rate_tyiyn_per_started_minute = EXCLUDED.tariff_driving_rate_tyiyn_per_started_minute,
    tariff_paused_rate_tyiyn_per_started_minute = EXCLUDED.tariff_paused_rate_tyiyn_per_started_minute,
    tariff_version = EXCLUDED.tariff_version,
    version = rentals.version + 1
RETURNING version`

// restoredVersion is the version a prepared rental is created with. It is the starting point of a
// sequence rather than a value that means anything on its own.
const restoredVersion = 1

// restoreRental puts one prepared rental back and reports the version it reached.
func (s store) restoreRental(ctx context.Context, prepared preparedRental) (int64, error) {
	var version int64
	err := s.querier(ctx).QueryRow(ctx, restoreRentalStatement,
		prepared.id,
		prepared.userID,
		prepared.vehicleID,
		prepared.stage,
		prepared.tariffID,
		prepared.zoneID,
		rentals.ReservationLifetime.Seconds(),
		prepared.stage != stage.Reserved,
		restoredVersion,
	).Scan(&version)
	return version, err
}

// lockScenarioVehicles takes the scenario vehicles for update. A rental references its vehicle, so
// inserting one takes a conflicting lock on that row: holding these locks first is what keeps a
// rental started at the same moment from slipping past the conflict check below.
const lockScenarioVehiclesStatement = `
SELECT id FROM vehicles WHERE id = ANY($1) ORDER BY id FOR UPDATE`

func (s store) lockScenarioVehicles(ctx context.Context, vehicleIDs []string) error {
	_, err := s.querier(ctx).Exec(ctx, lockScenarioVehiclesStatement, vehicleIDs)
	return err
}

// vehiclesRentedByPeople finds the scenario vehicles a rental outside the scenario accounts
// touches, whether that rental is still live or already over.
const vehiclesRentedByPeopleStatement = `
SELECT DISTINCT rental.vehicle_id
FROM rentals rental
JOIN users account ON account.id = rental.user_id
WHERE rental.vehicle_id = ANY($1) AND account.email <> ALL($2)
ORDER BY rental.vehicle_id`

func (s store) vehiclesRentedByPeople(
	ctx context.Context, vehicleIDs, scenarioAddresses []string,
) ([]string, error) {
	rows, err := s.querier(ctx).Query(ctx, vehiclesRentedByPeopleStatement, vehicleIDs, scenarioAddresses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicting []string
	for rows.Next() {
		var vehicleID string
		if err = rows.Scan(&vehicleID); err != nil {
			return nil, err
		}
		conflicting = append(conflicting, vehicleID)
	}
	return conflicting, rows.Err()
}

// The rentals a restoration removes are the ones it is about to put back, so that a prepared rental
// keeps its row and its version: everything else on a scenario vehicle belongs to the scenario as
// well, and a person's rental on one of them has already stopped the command.
const deleteScenarioRentalsStatement = `
DELETE FROM rentals WHERE vehicle_id = ANY($1) AND NOT (id = ANY($2))`

func (s store) deleteScenarioRentals(ctx context.Context, vehicleIDs, preparedIDs []string) error {
	_, err := s.querier(ctx).Exec(ctx, deleteScenarioRentalsStatement, vehicleIDs, preparedIDs)
	return err
}

// A restored vehicle is published differently: its link, its reserves, the age of its reading and
// whether it is in service are what the catalog shows. Its version is raised rather than set, so
// putting a vehicle back never makes its sequence move backwards.
//
// A vehicle is returned to service by the restoration because the scenario states it as one a person
// may book: a ride that ran out took it out of service, and putting the scenario back is the explicit
// servicing that answers that.
const restoreVehicleStatement = `
UPDATE vehicles
SET connected = $2, reporting = $3, route_id = $4, service_required = false, version = version + 1
WHERE id = $1
RETURNING version`

const restoreEnergySourceStatement = `
UPDATE vehicle_energy_sources
SET capacity = $3::numeric, remaining = $4::numeric
WHERE vehicle_id = $1 AND source_kind = $2`

const restoreTelemetryStatement = `
UPDATE vehicle_telemetry
SET position = ST_SetSRID(ST_MakePoint($2, $3), $4),
    confirmed_at = now() - make_interval(secs => $5)
WHERE vehicle_id = $1`

// restoreVehicle puts one prepared vehicle back where it stood, with the reserves it started from
// and the link it demonstrates, and reports the version the restoration published.
func (s store) restoreVehicle(ctx context.Context, vehicle Vehicle) (int64, error) {
	querier := s.querier(ctx)
	var version int64
	if err := querier.QueryRow(ctx, restoreVehicleStatement,
		vehicle.ID, vehicle.Connected, vehicle.Reporting, string(vehicle.RouteID)).Scan(&version); err != nil {
		return 0, err
	}
	for _, source := range vehicle.Sources {
		_, err := querier.Exec(ctx, restoreEnergySourceStatement,
			vehicle.ID, source.Kind, source.Capacity.Decimal(), source.Remaining.Decimal())
		if err != nil {
			return 0, err
		}
	}
	_, err := querier.Exec(ctx, restoreTelemetryStatement,
		vehicle.ID, vehicle.Position.Longitude, vehicle.Position.Latitude, fleet.WGS84SRID,
		vehicle.confirmedAgo().Seconds())
	return version, err
}
