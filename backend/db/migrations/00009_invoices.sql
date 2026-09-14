-- +goose Up
-- A rental that has ended says why it ended. The reason is one of the two the contract publishes as
-- the completion of a ride, and `energy_depleted` is admitted here because the rule that writes it
-- belongs to the same column; a ride this build cannot end is not one this build can complete.
ALTER TABLE rentals
    ADD COLUMN completion_reason text
        CHECK (completion_reason IN ('user_finished', 'energy_depleted'));

ALTER TABLE rentals
    -- Only a ride that was ended states why it ended. A reservation given back or run out released its
    -- vehicle without one, so the reason is required exactly of the stage a finish reaches.
    ADD CONSTRAINT rentals_completion_reason_with_stage
        CHECK ((completion_reason IS NOT NULL) = (stage = 'completed'));

-- An invoice is what a ride cost, written down once and never again. It is created by the same
-- transaction that ends the ride, so an invoice cannot describe a ride that did not finish, and a
-- ride cannot finish without one.
--
-- The identifiers of the two modes are columns rather than rows of a second table: the contract fixes
-- exactly two lines, driving then paused, including the modes the ride never entered. A row per line
-- would be a second declaration of a list the contract already states, and it could hold a third.
CREATE TABLE invoices (
    id uuid PRIMARY KEY,
    -- One invoice per rental is the whole of "a repeated finish issues nothing": the database decides
    -- it rather than a read two writers could both find nothing in.
    rental_id uuid NOT NULL UNIQUE REFERENCES rentals (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id),
    -- Written by the transaction that fixes the moment, never by the row's own insert: every moment
    -- this table holds is one the database read after the locks of the transaction that wrote it.
    issued_at timestamptz NOT NULL,
    currency text NOT NULL CHECK (currency = 'KGS'),
    billing_policy text NOT NULL CHECK (billing_policy = 'per_mode_started_minute_v1'),
    completion_reason text NOT NULL
        CHECK (completion_reason IN ('user_finished', 'energy_depleted')),
    driving_duration_microseconds bigint NOT NULL CHECK (driving_duration_microseconds >= 0),
    paused_duration_microseconds bigint NOT NULL CHECK (paused_duration_microseconds >= 0),
    driving_billed_started_minutes bigint NOT NULL CHECK (driving_billed_started_minutes >= 0),
    paused_billed_started_minutes bigint NOT NULL CHECK (paused_billed_started_minutes >= 0),
    -- The rates are copied from the rental rather than read through it. An invoice states what it was
    -- issued at, and a later change of the rental must not change what a stored invoice says.
    driving_rate_tyiyn_per_started_minute bigint NOT NULL
        CHECK (driving_rate_tyiyn_per_started_minute >= 0),
    paused_rate_tyiyn_per_started_minute bigint NOT NULL
        CHECK (paused_rate_tyiyn_per_started_minute >= 0),
    total_amount_tyiyn bigint NOT NULL CHECK (total_amount_tyiyn >= 0),
    version bigint NOT NULL CHECK (version > 0)
);

-- An account reads its own invoices newest first, and the index is ordered the way that read walks.
CREATE INDEX invoices_owner_idx ON invoices (user_id, issued_at DESC, id DESC);

-- What an invoice's payment is. A zero invoice is paid the moment it is issued and a positive one
-- starts pending; the transitions between the two belong to the payment task, which is why the
-- statuses are admitted here rather than added later. The version records how many times the
-- published view of the payment changed, exactly as a vehicle's and a rental's do.
CREATE TABLE invoice_payments (
    invoice_id uuid PRIMARY KEY REFERENCES invoices (id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('pending', 'failed', 'paid')),
    version bigint NOT NULL CHECK (version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    paid_at timestamptz,
    failed_at timestamptz,
    failure_code text CHECK (failure_code IN ('declined')),
    CHECK (updated_at >= created_at),
    -- Each status carries exactly the moments it can have, so a view cannot be built from a
    -- combination no transition produces.
    CHECK ((status = 'paid') = (paid_at IS NOT NULL)),
    CHECK ((status = 'failed') = (failed_at IS NOT NULL)),
    CHECK ((failure_code IS NULL) = (failed_at IS NULL))
);

-- The application issues an invoice and reads it back, and never changes one: immutability is a
-- privilege rather than only a promise. The payment beside it is written with the invoice and read
-- from it; the privilege to move a payment belongs to the task that owns that transition.
GRANT SELECT, INSERT ON invoices TO carsharing_app;
GRANT SELECT, INSERT ON invoice_payments TO carsharing_app;

-- +goose Down
DROP TABLE invoice_payments;
DROP TABLE invoices;

ALTER TABLE rentals
    DROP CONSTRAINT rentals_completion_reason_with_stage,
    DROP COLUMN completion_reason;
