-- +goose Up
-- 00009 required every rental that had released its vehicle to state why it ended, which is not what
-- an ending is: a reservation given back or run out releases its vehicle without a reason, and only a
-- ride that was ended has one. The constraint is replaced by the one the rule actually is.
--
-- 00009 is corrected for a database created from scratch as well, so that constraint no longer exists
-- anywhere; this migration is what repairs a database that applied the earlier form, and its drop is
-- conditional for the database that never had it. An applied migration is never edited.
--
-- +goose StatementBegin
DO $$
BEGIN
    ALTER TABLE rentals DROP CONSTRAINT IF EXISTS rentals_completion_reason_with_end;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'rentals_completion_reason_with_stage'
          AND conrelid = 'rentals'::regclass
    ) THEN
        ALTER TABLE rentals
            ADD CONSTRAINT rentals_completion_reason_with_stage
                CHECK ((completion_reason IS NOT NULL) = (stage = 'completed'));
    END IF;
END
$$
-- +goose StatementEnd

-- +goose Down
ALTER TABLE rentals
    DROP CONSTRAINT IF EXISTS rentals_completion_reason_with_stage;

ALTER TABLE rentals
    ADD CONSTRAINT rentals_completion_reason_with_end
        CHECK ((completion_reason IS NULL) = (ended_at IS NULL));
