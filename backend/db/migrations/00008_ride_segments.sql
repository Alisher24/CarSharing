-- +goose Up
-- A ride is a sequence of intervals, each in one mode. The intervals are the only record of how long a
-- vehicle drove and how long it stood still, so the durations and the amount are summed from them
-- rather than from a running total that a second writer could disagree with.
--
-- mode_started_at is the moment the current mode began. It is stated once on the rental because it is
-- published there, and it always names the open interval below: one transition writes both.
ALTER TABLE rentals
    ADD COLUMN mode_started_at timestamptz;

-- A ride that had already begun before this migration has no record of when its current mode began,
-- and no interval to recover it from. Its own start is the closest true statement: the first mode of a
-- ride begins when the ride does. The backfill runs before the constraint below, so a database that
-- already holds rides migrates rather than failing on them.
UPDATE rentals
SET mode_started_at = started_at
WHERE stage IN ('active', 'paused') AND mode_started_at IS NULL AND started_at IS NOT NULL;

ALTER TABLE rentals
    -- A ride that has started has a mode moment, and one that has not has none. Saying it here keeps a
    -- row from carrying a moment that no interval explains.
    ADD CONSTRAINT rentals_mode_started_with_ride
        CHECK ((mode_started_at IS NOT NULL) = (stage IN ('active', 'paused')));

CREATE TABLE ride_segments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- An interval is a fact about one ride and has no meaning without it.
    rental_id uuid NOT NULL REFERENCES rentals (id) ON DELETE CASCADE,
    mode text NOT NULL CHECK (mode IN ('driving', 'paused')),
    -- Written by the transaction that fixes the moment, never by the row's own insert: every moment
    -- this table holds is one the database read after the locks of the transition that wrote it.
    started_at timestamptz NOT NULL,
    -- Open intervals have no end. The ride is in the mode of its one open interval, so an interval is
    -- closed by the same statement that opens the next one.
    ended_at timestamptz,
    -- A mode that lasts no time is not an interval. Two transitions of one ride run under the lock of
    -- their rental, so the closing moment is always strictly later than the opening one.
    CHECK (ended_at IS NULL OR ended_at > started_at)
);

-- One open interval per ride, decided by the database rather than by a read before an insert: two
-- transactions that both decided to open one cannot both succeed.
CREATE UNIQUE INDEX ride_segments_one_open_per_rental
    ON ride_segments (rental_id)
    WHERE ended_at IS NULL;

-- The intervals of one ride are read oldest first, which is the order they were opened in.
CREATE INDEX ride_segments_ride_idx ON ride_segments (rental_id, started_at);

-- The transitions read, close and open intervals, and never rewrite a closed one.
GRANT SELECT, INSERT, UPDATE ON ride_segments TO carsharing_app;

-- +goose Down
DROP TABLE ride_segments;

ALTER TABLE rentals
    DROP CONSTRAINT rentals_mode_started_with_ride,
    DROP COLUMN mode_started_at;
