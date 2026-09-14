-- +goose Up
-- 00009 required every rental that had released its vehicle to state why it ended, which is not what
-- an ending is: a reservation given back or run out releases its vehicle without a reason, and only a
-- ride that was ended has one. The constraint is replaced by the one the rule actually is.
--
-- 00009 is corrected for a database created from scratch as well; this migration is what repairs a
-- database that already applied the earlier form, because an applied migration is never edited.
ALTER TABLE rentals
    DROP CONSTRAINT rentals_completion_reason_with_end;

ALTER TABLE rentals
    ADD CONSTRAINT rentals_completion_reason_with_stage
        CHECK ((completion_reason IS NOT NULL) = (stage = 'completed'));

-- +goose Down
ALTER TABLE rentals
    DROP CONSTRAINT rentals_completion_reason_with_stage;

ALTER TABLE rentals
    ADD CONSTRAINT rentals_completion_reason_with_end
        CHECK ((completion_reason IS NULL) = (ended_at IS NULL));
