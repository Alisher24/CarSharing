-- +goose Up
-- One durable thing the service has to tell one account: a warning that a reservation is running
-- out, and later the report of a finished ride. The unique key on rental and kind is the whole of
-- "exactly once": the database decides it rather than a check that two writers could both pass, and
-- a repeated attempt writes nothing, so the moment the first one stored is never replaced.
--
-- History is kept. Leaving the reservation makes the record inactive instead of deleting it, and the
-- version records how many times its published representation changed.
CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id),
    -- A notification is about a rental and publishes that rental's deadline, so it cannot outlive it.
    rental_id uuid NOT NULL REFERENCES rentals (id) ON DELETE CASCADE,
    -- The two kinds the contract declares. The storage already admits both, so the task that starts
    -- writing the second one adds a value rather than a migration of this table.
    kind text NOT NULL CHECK (kind IN ('reservation_expiring', 'rental_completed')),
    -- Written by the transaction that fixes the moment, never by the row's own insert: every moment
    -- this table holds is one the database read after the locks of the transaction that wrote it.
    created_at timestamptz NOT NULL,
    read_at timestamptz,
    active boolean NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    CHECK (read_at IS NULL OR read_at >= created_at)
);

-- The owner's collection is read newest first with the identifier as its tie-break, and the index is
-- ordered the way that read walks it.
CREATE INDEX notifications_owner_idx ON notifications (user_id, created_at DESC, id DESC);

-- The application writes a notification, reads it back and changes it, and never deletes one: a
-- warning that stopped being current stays in the history it belongs to.
GRANT SELECT, INSERT, UPDATE ON notifications TO carsharing_app;

-- +goose Down
DROP TABLE notifications;
