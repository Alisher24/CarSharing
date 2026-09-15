-- +goose Up
-- An account reads its own finished rides newest first, and the index is ordered the way that read
-- walks: the moment the ride ended, with the identifier breaking ties between rides that ended at the
-- same one. It covers the completed stage alone, because a reservation given back or run out is never
-- part of the history and a live rental is what the current read answers.
CREATE INDEX rentals_completed_history_idx
    ON rentals (user_id, ended_at DESC, id DESC)
    WHERE stage = 'completed';

-- +goose Down
DROP INDEX rentals_completed_history_idx;
