-- +goose Up
-- Counters live beside the sessions in PostgreSQL rather than in the API process, so restarting
-- the service does not hand an attacker a fresh budget. One row per counted subject; the window
-- start is stored so that the wait advertised to a caller is the real remaining time.
CREATE TABLE rate_limit_counters (
    scope text NOT NULL,
    subject text NOT NULL,
    window_started_at timestamptz NOT NULL,
    attempts integer NOT NULL CHECK (attempts > 0),
    PRIMARY KEY (scope, subject)
);

-- Expired rows are rewritten in place by the next attempt on the same subject; this index lets a
-- future cleanup find the ones no attempt ever returns to.
CREATE INDEX rate_limit_counters_window_idx ON rate_limit_counters (window_started_at);

GRANT SELECT, INSERT, UPDATE, DELETE ON rate_limit_counters TO carsharing_app;

-- +goose Down
DROP TABLE rate_limit_counters;
