-- +goose Up
-- The counter states how many attempts a window had decided before the claim that last answered it, so
-- the claim that opens a window writes zero: the row is created by the first attempt of a window and
-- carries the moment that window began rather than stating that an attempt was spent. 00003 allowed a
-- row to state one attempt at least, which is what a counter that counted in that shape held; it is
-- corrected here rather than in place, because 00003 has been applied.
ALTER TABLE rate_limit_counters
    DROP CONSTRAINT rate_limit_counters_attempts_check;

ALTER TABLE rate_limit_counters
    ADD CONSTRAINT rate_limit_counters_attempts_check CHECK (attempts >= 0);

-- +goose Down
-- A counter holding no attempt states a window nothing was spent in, which the older shape has no way
-- of stating: the row is removed rather than rewritten, exactly as the code that gave the attempt back
-- would have removed it.
DELETE FROM rate_limit_counters WHERE attempts = 0;

ALTER TABLE rate_limit_counters
    DROP CONSTRAINT rate_limit_counters_attempts_check;

ALTER TABLE rate_limit_counters
    ADD CONSTRAINT rate_limit_counters_attempts_check CHECK (attempts > 0);
