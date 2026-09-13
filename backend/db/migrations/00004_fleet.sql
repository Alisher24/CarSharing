-- +goose Up
CREATE TABLE vehicles (
    id uuid PRIMARY KEY,
    model text NOT NULL CHECK (model <> ''),
    powertrain_type text NOT NULL
        CHECK (powertrain_type IN ('electric', 'gasoline', 'diesel', 'hybrid', 'gas')),
    -- connected is the vehicle's link to the platform; reporting is whether it currently sends
    -- telemetry. A vehicle can be linked and silent, which is how a position goes stale without
    -- the vehicle being reported as offline.
    connected boolean NOT NULL,
    reporting boolean NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE vehicle_energy_sources (
    vehicle_id uuid NOT NULL REFERENCES vehicles (id),
    source_kind text NOT NULL
        CHECK (source_kind IN ('battery', 'gasoline', 'diesel', 'lpg', 'cng')),
    capacity numeric(14, 6) NOT NULL CHECK (capacity > 0),
    remaining numeric(14, 6) NOT NULL CHECK (remaining >= 0),
    CHECK (remaining <= capacity),
    PRIMARY KEY (vehicle_id, source_kind)
);

-- The last confirmed reading, which is the only position the public catalog publishes. A vehicle
-- that has never reported has no row here and therefore no public position.
CREATE TABLE vehicle_telemetry (
    vehicle_id uuid PRIMARY KEY REFERENCES vehicles (id),
    position geometry(Point, 4326) NOT NULL,
    confirmed_at timestamptz NOT NULL
);

CREATE TABLE service_zones (
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (name <> ''),
    -- The contract publishes a zone as a polygon or a set of them, so the column admits nothing
    -- else and refuses geometry that no coverage test could answer.
    area geometry(Geometry, 4326) NOT NULL
        CHECK (ST_IsValid(area) AND GeometryType(area) IN ('POLYGON', 'MULTIPOLYGON')),
    version bigint NOT NULL CHECK (version > 0)
);

CREATE TABLE tariffs (
    id uuid PRIMARY KEY,
    currency text NOT NULL CHECK (currency = 'KGS'),
    billing_policy text NOT NULL CHECK (billing_policy = 'per_mode_started_minute_v1'),
    driving_rate_tyiyn_per_started_minute bigint NOT NULL
        CHECK (driving_rate_tyiyn_per_started_minute >= 0),
    paused_rate_tyiyn_per_started_minute bigint NOT NULL
        CHECK (paused_rate_tyiyn_per_started_minute >= 0),
    version bigint NOT NULL CHECK (version > 0)
);

-- A reservation and the ride it becomes are stages of one rental, so a single partial unique index
-- can refuse a second rental for the same vehicle or the same person. ended_at is that predicate:
-- while it is null the rental still holds its vehicle, whichever stage it is in.
CREATE TABLE rentals (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id),
    vehicle_id uuid NOT NULL REFERENCES vehicles (id),
    stage text NOT NULL
        CHECK (stage IN ('reserved', 'active', 'paused', 'cancelled', 'expired', 'completed')),
    tariff_id uuid NOT NULL REFERENCES tariffs (id),
    zone_id uuid NOT NULL REFERENCES service_zones (id),
    reserved_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    started_at timestamptz,
    ended_at timestamptz,
    CHECK ((ended_at IS NULL) = (stage IN ('reserved', 'active', 'paused')))
);
CREATE UNIQUE INDEX rentals_one_live_per_vehicle ON rentals (vehicle_id) WHERE ended_at IS NULL;
CREATE UNIQUE INDEX rentals_one_live_per_user ON rentals (user_id) WHERE ended_at IS NULL;
CREATE INDEX rentals_due_reservations ON rentals (expires_at) WHERE stage = 'reserved';

GRANT SELECT ON vehicles, vehicle_energy_sources, service_zones, tariffs, rentals TO carsharing_app;
-- The demo telemetry source and the reservation expiry both run inside the API, so the application
-- role writes exactly the two facts they own and nothing else.
GRANT SELECT, INSERT, UPDATE ON vehicle_telemetry TO carsharing_app;
GRANT UPDATE ON rentals TO carsharing_app;

-- +goose Down
DROP TABLE rentals;
DROP TABLE vehicle_telemetry;
DROP TABLE vehicle_energy_sources;
DROP TABLE vehicles;
DROP TABLE tariffs;
DROP TABLE service_zones;
