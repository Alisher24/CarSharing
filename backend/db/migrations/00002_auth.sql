-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- The primary key is the SHA-256 hash of the session token, never the token itself, so a disclosure
-- of this table does not hand an attacker sessions it can present. Columns follow the names the
-- session library writes; the application never reads the payload column directly.
CREATE TABLE sessions (
    token text PRIMARY KEY,
    data bytea NOT NULL,
    expiry timestamptz NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

GRANT SELECT, INSERT ON users TO carsharing_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON sessions TO carsharing_app;

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
