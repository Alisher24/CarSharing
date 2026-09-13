-- +goose Up
-- The conditions a reservation was made under belong to the rental, not to the price list it was
-- read from: a later catalog change must not reach a reservation that already exists. The columns
-- are added with a default so the rows that already exist can be filled from their price list, and
-- the default is then dropped so no later insert can leave them out.
ALTER TABLE rentals
    ADD COLUMN tariff_currency text NOT NULL DEFAULT 'KGS',
    ADD COLUMN tariff_billing_policy text NOT NULL DEFAULT 'per_mode_started_minute_v1',
    ADD COLUMN tariff_driving_rate_tyiyn_per_started_minute bigint NOT NULL DEFAULT 0
        CHECK (tariff_driving_rate_tyiyn_per_started_minute >= 0),
    ADD COLUMN tariff_paused_rate_tyiyn_per_started_minute bigint NOT NULL DEFAULT 0
        CHECK (tariff_paused_rate_tyiyn_per_started_minute >= 0),
    ADD COLUMN tariff_version bigint NOT NULL DEFAULT 1 CHECK (tariff_version > 0);

UPDATE rentals
SET tariff_currency = tariff.currency,
    tariff_billing_policy = tariff.billing_policy,
    tariff_driving_rate_tyiyn_per_started_minute = tariff.driving_rate_tyiyn_per_started_minute,
    tariff_paused_rate_tyiyn_per_started_minute = tariff.paused_rate_tyiyn_per_started_minute,
    tariff_version = tariff.version
FROM tariffs tariff
WHERE tariff.id = rentals.tariff_id;

ALTER TABLE rentals
    ALTER COLUMN tariff_currency DROP DEFAULT,
    ALTER COLUMN tariff_billing_policy DROP DEFAULT,
    ALTER COLUMN tariff_driving_rate_tyiyn_per_started_minute DROP DEFAULT,
    ALTER COLUMN tariff_paused_rate_tyiyn_per_started_minute DROP DEFAULT,
    ALTER COLUMN tariff_version DROP DEFAULT;

-- One mutating command, remembered by its key for the account that sent it. The row is written
-- inside the transaction that makes the change, so a repeat that arrives while the first attempt is
-- still running waits on this row rather than performing the change twice, and a repeated command
-- whose response was lost answers what the first attempt answered.
--
-- A row that is visible to another transaction always carries its result: status_code, body and the
-- repeat window are written by the same commit that writes the change. claimed_at is therefore a
-- diagnostic of the attempt, not a state another request has to interpret.
CREATE TABLE idempotency_requests (
    user_id uuid NOT NULL REFERENCES users (id),
    command_key uuid NOT NULL,
    fingerprint text NOT NULL CHECK (fingerprint <> ''),
    claimed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    status_code integer CHECK (status_code BETWEEN 100 AND 599),
    body jsonb,
    completed_at timestamptz,
    retain_until timestamptz,
    PRIMARY KEY (user_id, command_key),
    CHECK ((status_code IS NULL) = (body IS NULL)),
    CHECK ((completed_at IS NULL) = (status_code IS NULL)),
    CHECK ((retain_until IS NULL) = (completed_at IS NULL)),
    -- The response describes the command that was answered rather than the rental it moved, so it
    -- is kept well past the rental's own life: a client may replay it long after the reservation
    -- has been cancelled or has run out.
    CHECK (retain_until IS NULL OR retain_until >= completed_at)
);

CREATE INDEX idempotency_requests_retention_idx ON idempotency_requests (retain_until)
    WHERE retain_until IS NOT NULL;

-- The API claims, answers and expires these rows; nothing else reads them.
GRANT SELECT, INSERT, UPDATE, DELETE ON idempotency_requests TO carsharing_app;

-- A command locks the account it belongs to before it decides anything: the day's allowance and the
-- one live rental of a person are both decided under that lock, so two requests cannot both find the
-- day unspent. PostgreSQL requires the update privilege to take a row lock, so the application role
-- is given it here; nothing in the application updates an account.
GRANT UPDATE ON users TO carsharing_app;

-- Creating a reservation writes a rental, which earlier tasks only ever updated. The columns that
-- hold the conditions it was made under are written by the same statement, so the one privilege
-- covers both.
GRANT INSERT ON rentals TO carsharing_app;

-- +goose Down
DROP TABLE idempotency_requests;

ALTER TABLE rentals
    DROP COLUMN tariff_currency,
    DROP COLUMN tariff_billing_policy,
    DROP COLUMN tariff_driving_rate_tyiyn_per_started_minute,
    DROP COLUMN tariff_paused_rate_tyiyn_per_started_minute,
    DROP COLUMN tariff_version;
