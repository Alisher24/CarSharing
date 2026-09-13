-- +goose Up
-- A rental carries a version for the same reason a vehicle does: a signal about it names the version
-- it changed to, and a reader compares versions within one resource. The column is added with a
-- starting value so an existing row keeps a sequence instead of appearing to move backwards.
ALTER TABLE rentals ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);

-- One durable piece of work: a change that was committed together with the domain change it
-- describes, and that a worker must still deliver. kind is deliberately unrestricted text: a task
-- whose type this build does not know is kept for diagnostics rather than refused at insert time.
CREATE TABLE outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind <> ''),
    resource_id uuid NOT NULL,
    version bigint NOT NULL CHECK (version >= 0),
    -- NULL is a change every reader may hear about; a value addresses it to the one account it
    -- belongs to, which is what keeps a private signal out of another session's stream.
    recipient_id uuid REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    -- attempts counts the claims a task has had, so a task that keeps failing stays visible.
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text,
    -- next_attempt_at is when the task may be claimed again; lease_token names the attempt that
    -- currently owns it, and only that token may complete or fail it before the lease runs out.
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_token uuid,
    lease_expires_at timestamptz,
    completed_at timestamptz,
    CHECK ((lease_token IS NULL) = (lease_expires_at IS NULL))
);

-- A worker claims the due task with the earliest next_attempt_at, so the index is ordered the way
-- that claim reads. A completed task is never claimed again and is left out of the index.
CREATE INDEX outbox_due_idx ON outbox (next_attempt_at, id) WHERE completed_at IS NULL;
-- Retention deletes delivered signal tasks by completion time, in small batches.
CREATE INDEX outbox_completed_idx ON outbox (completed_at) WHERE completed_at IS NOT NULL;
-- The worker records, claims and settles tasks; the API records the changes of the demonstration
-- telemetry source and confirms that source's readings, which raises a vehicle's version.
GRANT SELECT, INSERT, UPDATE, DELETE ON outbox TO carsharing_app;
GRANT UPDATE ON vehicles TO carsharing_app;

-- +goose Down
DROP TABLE outbox;
ALTER TABLE rentals DROP COLUMN version;
