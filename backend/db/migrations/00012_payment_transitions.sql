-- +goose Up
-- The right to move a payment is granted by column, so a transition writes the state of a payment and
-- the moments that state carries and nothing else: the invoice a payment belongs to and the moment it
-- was created are the two facts that make it the payment of that invoice, and neither of them is
-- something a transition has any business rewriting. The invoice table itself stays one the
-- application only reads and inserts.
GRANT UPDATE (status, version, updated_at, paid_at, failed_at, failure_code)
    ON invoice_payments TO carsharing_app;

-- One demand for the outcome of the next payment attempt of one ride: what a demonstration asks for so
-- that a decline can be shown at all. It is bound to the ride rather than to the installation, so two
-- demonstrations running at once do not decide each other's outcome.
--
-- The demand is spent by the attempt it decided, in the same transaction as the transition it decided:
-- a rolled back attempt leaves it in place, and a redelivered task cannot make it decide twice.
CREATE TABLE demo_payment_outcomes (
    rental_id uuid PRIMARY KEY REFERENCES rentals (id) ON DELETE CASCADE,
    outcome text NOT NULL CHECK (outcome IN ('paid', 'failed')),
    set_at timestamptz NOT NULL
);

-- The application reads a demand and spends it, and cannot make one: the privilege is what keeps the
-- protected demonstration scenario protected, because only the migrator role can write this table.
GRANT SELECT, DELETE ON demo_payment_outcomes TO carsharing_app;

-- +goose Down
DROP TABLE demo_payment_outcomes;

REVOKE UPDATE (status, version, updated_at, paid_at, failed_at, failure_code)
    ON invoice_payments FROM carsharing_app;
