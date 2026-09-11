-- +goose Up
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE bootstrap_metadata (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    city text NOT NULL,
    currency text NOT NULL CHECK (currency = 'KGS'),
    timezone text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO bootstrap_metadata (city, currency, timezone)
VALUES ('Бишкек', 'KGS', 'Asia/Bishkek');

CREATE TABLE seed_runs (
    name text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

GRANT USAGE ON SCHEMA public TO carsharing_app;
GRANT SELECT ON bootstrap_metadata TO carsharing_app;

-- +goose Down
DROP TABLE seed_runs;
DROP TABLE bootstrap_metadata;
-- Keep the shared PostGIS extension and provisioned roles intact.
