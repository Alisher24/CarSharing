-- +goose Up
-- The notifications table already admits both kinds the contract declares, and this task starts
-- writing the second one: the report of a finished ride carries the invoice it produced.
ALTER TABLE notifications
    ADD COLUMN invoice_id uuid REFERENCES invoices (id) ON DELETE CASCADE;

ALTER TABLE notifications
    ADD COLUMN completion_reason text
        CHECK (completion_reason IN ('user_finished', 'energy_depleted'));

ALTER TABLE notifications
    -- A warning about a reservation is about a deadline and carries no invoice, and the report of a
    -- finished ride carries the invoice and the reason it ended. Stating it here keeps a row from
    -- being a shape no kind declares.
    ADD CONSTRAINT notifications_invoice_with_completion
        CHECK ((kind = 'rental_completed') = (invoice_id IS NOT NULL)),
    ADD CONSTRAINT notifications_reason_with_completion
        CHECK ((kind = 'rental_completed') = (completion_reason IS NOT NULL));

-- +goose Down
ALTER TABLE notifications
    DROP CONSTRAINT notifications_invoice_with_completion,
    DROP CONSTRAINT notifications_reason_with_completion,
    DROP COLUMN invoice_id,
    DROP COLUMN completion_reason;
