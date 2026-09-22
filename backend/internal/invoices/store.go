package invoices

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInvoiceNotFound reports an invoice this account does not hold, whether no such invoice exists or
// it belongs to somebody else.
var ErrInvoiceNotFound = errors.New("no such invoice")

// Store is the invoice tables. Every statement runs on the querier the context carries, so an invoice
// commits together with the change that produced it or not at all.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// invoiceFields is the shape every read of an invoice answers with: the invoice, its two lines and the
// state of its payment. One declaration keeps a read by rental and a read by identifier from drifting
// apart.
const invoiceFields = `
    invoice.id,
    invoice.rental_id,
    invoice.user_id,
    invoice.issued_at,
    invoice.currency,
    invoice.billing_policy,
    invoice.completion_reason,
    invoice.exhausted_sources,
    invoice.version,
    invoice.driving_duration_microseconds,
    invoice.driving_billed_started_minutes,
    invoice.driving_rate_tyiyn_per_started_minute,
    invoice.paused_duration_microseconds,
    invoice.paused_billed_started_minutes,
    invoice.paused_rate_tyiyn_per_started_minute,
    invoice.total_amount_tyiyn,
    payment.status,
    payment.version,
    payment.updated_at,
    payment.paid_at,
    payment.failed_at,
    payment.failure_code`

// invoiceColumns reads the payment beside the invoice, because the contract publishes the two together
// and an invoice is always issued with one.
const invoiceColumns = `
SELECT` + invoiceFields + `
FROM invoices invoice
JOIN invoice_payments payment ON payment.invoice_id = invoice.id`

const (
	invoiceOfRentalSelection = invoiceColumns + `
WHERE invoice.rental_id = $1`

	invoiceByIDForSelection = invoiceColumns + `
WHERE invoice.id = $1 AND invoice.user_id = $2`

	invoiceByIDSelection = invoiceColumns + `
WHERE invoice.id = $1`
)

// Issue writes one invoice for one finished ride and answers it as it was stored.
//
// The identifier is drawn here and the row is read back through the selection every reader uses, so
// what an issuance answers is what a later read of the same invoice answers. The statement copies the
// currency and the policy from the rental rather than taking them from the draft: an invoice states the
// conditions its ride was reserved under, and a caller cannot hand it others by mistake.
func (s *Store) Issue(ctx context.Context, draft Draft) (Invoice, error) {
	if err := draft.Validate(); err != nil {
		return Invoice{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Invoice{}, err
	}
	if err = s.insert(ctx, id.String(), draft); err != nil {
		return Invoice{}, err
	}
	return s.ByID(ctx, draft.UserID, id.String())
}

// insertLine is one line of an invoice as the statement below is given it. Every value is a plain
// int64: a named type of the same width is not one the driver is obliged to send as an integer, and a
// number that reached the database as a double would have lost the last digits of a rate or the
// microseconds of a duration before the column was ever written.
type insertLine struct {
	durationMicroseconds int64
	billedMinutes        int64
	rateTyiynPerMinute   int64
}

// insertLineOf reduces one line of a draft to the plain whole numbers the statement is given.
func insertLineOf(line Line) insertLine {
	return insertLine{
		durationMicroseconds: int64(line.Duration),
		billedMinutes:        line.Minutes,
		rateTyiynPerMinute:   int64(line.Rate),
	}
}

// insert writes the invoice and the first state of its payment, one statement each: an invoice without
// a payment is a view the contract cannot publish, so the two are written together.
func (s *Store) insert(ctx context.Context, id string, draft Draft) error {
	driving, paused := insertLineOf(draft.Driving), insertLineOf(draft.Paused)
	status, paidAt := FirstPayment(draft.TotalTyiyn, draft.IssuedAt)
	querier := database.QuerierFrom(ctx, s.pool)
	written, err := querier.Exec(ctx, insertInvoiceStatement,
		id,
		draft.UserID,
		draft.IssuedAt,
		draft.Completion,
		fleet.SourceNames(draft.Exhausted),
		driving.durationMicroseconds,
		driving.billedMinutes,
		driving.rateTyiynPerMinute,
		paused.durationMicroseconds,
		paused.billedMinutes,
		paused.rateTyiynPerMinute,
		int64(draft.TotalTyiyn),
		database.InitialVersion,
		draft.RentalID,
	)
	if err != nil {
		return err
	}
	if written.RowsAffected() == 0 {
		// The selection matched no rental, so there is no ride to invoice. A ride removed while its
		// ending ran is the only way here, and writing the payment alone would leave a payment about
		// nothing.
		return ErrInvoiceNotFound
	}
	_, err = querier.Exec(ctx, insertPaymentStatement, id, status, database.InitialVersion,
		draft.IssuedAt, paidAt)
	return err
}

// ByRental reads the invoice of one ride. A ride that has not been invoiced is reported as absent
// rather than answered with an empty invoice.
func (s *Store) ByRental(ctx context.Context, rentalID string) (Invoice, error) {
	return s.read(ctx, invoiceOfRentalSelection, rentalID)
}

// ByID reads one invoice this account holds. Another account's invoice is reported as absent, so the
// answer cannot be used to learn that it exists.
func (s *Store) ByID(ctx context.Context, owner uuid.UUID, id string) (Invoice, error) {
	return s.read(ctx, invoiceByIDForSelection, id, owner)
}

// ByIDForRead reads one invoice and the ride it describes for anyone who may hold it. The contract
// states no read of an invoice of one's own, so this is not a published operation: it is how a command
// that has not yet established whose invoice it was handed names the two rows it must lock before it
// can decide anything, and the account that owns them.
//
// The read takes no lock, because it runs before the first one: a command locks the rows it planned
// for and proves afterwards that the relationships did not change.
func (s *Store) ByIDForRead(ctx context.Context, id string) (Invoice, string, error) {
	var rentalID string
	var owner uuid.UUID
	err := database.QuerierFrom(ctx, s.pool).QueryRow(ctx, invoiceOwnerStatement, id).
		Scan(&rentalID, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, "", ErrInvoiceNotFound
	}
	if err != nil {
		return Invoice{}, "", err
	}
	found, err := s.read(ctx, invoiceByIDSelection, id)
	return found, rentalID, err
}

// invoiceOwnerStatement names the ride an invoice describes and the account that owes it, without
// reading the invoice itself: a command needs the two rows to lock rather than the amount to display.
const invoiceOwnerStatement = `
SELECT rental_id, user_id
FROM invoices
WHERE id = $1`

// Outstanding reports whether this account owes money: whether it holds a positive invoice that no
// attempt has settled. It names the account rather than reading every invoice, because a debt is a
// property of one account and reading another's would be reading what this command has no business
// with.
//
// A zero invoice is not a debt: it is settled by the moment it was issued, so the payment state alone
// answers the question and the amount is not judged a second time here.
func (s *Store) Outstanding(ctx context.Context, owner uuid.UUID) (bool, error) {
	var owed bool
	err := database.QuerierFrom(ctx, s.pool).QueryRow(ctx, outstandingStatement, owner).Scan(&owed)
	return owed, err
}

const outstandingStatement = `
SELECT EXISTS (
    SELECT 1
    FROM invoices invoice
    JOIN invoice_payments payment ON payment.invoice_id = invoice.id
    WHERE invoice.user_id = $1
      AND invoice.total_amount_tyiyn > 0
      AND payment.status <> 'paid'
)`

// insertInvoiceStatement writes one invoice for one ride. The currency and the billing policy are
// selected from the rental rather than handed in by the caller, so an invoice cannot be issued under a
// price list its ride was never reserved under.
//
// Every number is cast to the type of its column. Without the cast the database is free to read a
// parameter as a floating-point number, and a rate of 9007199254740993 tyiyn would be stored as the
// double nearest to it while a duration would be rounded to whole milliseconds: an invoice is the one
// record of what a ride cost, so its digits are stated rather than inferred.
const insertInvoiceStatement = `
INSERT INTO invoices (
    id,
    rental_id,
    user_id,
    issued_at,
    currency,
    billing_policy,
    completion_reason,
    exhausted_sources,
    driving_duration_microseconds,
    driving_billed_started_minutes,
    driving_rate_tyiyn_per_started_minute,
    paused_duration_microseconds,
    paused_billed_started_minutes,
    paused_rate_tyiyn_per_started_minute,
    total_amount_tyiyn,
    version
)
SELECT $1::uuid,
       rental.id,
       $2::uuid,
       $3::timestamptz,
       rental.tariff_currency,
       rental.tariff_billing_policy,
       $4::text,
       $5::text[],
       $6::bigint,
       $7::bigint,
       $8::bigint,
       $9::bigint,
       $10::bigint,
       $11::bigint,
       $12::bigint,
       $13::bigint
FROM rentals rental
WHERE rental.id = $14::uuid`

// insertPaymentStatement writes the first state of a payment. The moment a settled payment states is
// the moment its invoice was issued, so a zero invoice is paid from the instant it exists; a payment
// that is not settled carries no moment of one, which the check of the table requires.
const insertPaymentStatement = `
INSERT INTO invoice_payments (invoice_id, status, version, created_at, updated_at, paid_at)
VALUES ($1, $2, $3, $4, $4, $5::timestamptz)`

// settleStatement moves the payment of one invoice from the state it must stand in to the one the
// attempt reached. Which state it starts from is part of the statement rather than of a read before
// it, so a second attempt at an invoice somebody has already settled moves no row and is answered as
// the refusal it is, whatever two attempts did at once.
//
// Each moment is written together with the state that carries it and the moments of the other state
// are cleared, so a payment never holds a moment its own state does not explain. The version counts
// the views this change produced, which is one per transition.
const settleStatement = `
UPDATE invoice_payments
SET status = $2::text,
    version = version + 1,
    updated_at = $3::timestamptz,
    paid_at = $4::timestamptz,
    failed_at = $5::timestamptz,
    failure_code = $6::text
WHERE invoice_id = $1 AND status = $7::text`

// Settle moves the payment of one invoice from the state the transition starts from to the state the
// attempt reached, and answers the invoice as the selection every reader uses reads it.
//
// The state it starts from is named by the caller because the transitions differ in what they are a
// repeat of: the first attempt applies to an invoice nothing has settled, and a manual attempt to one
// that was refused before. A transition that does not apply to the state the row stands in is reported
// as a refusal rather than as a successful write of nothing, and the row is left untouched.
//
// The transition and the read that answers it are two statements rather than one statement with a
// returning clause. A statement sees the snapshot its own start fixed, so the read inside it would
// answer the payment as it stood before the transition wrote it РІР‚вЂќ the answer would describe the state
// the attempt replaced while the row already held the state it reached.
func (s *Store) Settle(
	ctx context.Context, invoiceID string, from PaymentStatus, outcome SettleOutcome,
) (Invoice, error) {
	if err := outcome.Validate(); err != nil {
		return Invoice{}, err
	}
	columns := outcomeColumns(outcome)
	written, err := database.QuerierFrom(ctx, s.pool).Exec(ctx, settleStatement,
		invoiceID,
		outcome.Status,
		outcome.Moment,
		columns.paidAt,
		columns.failedAt,
		columns.failureCode,
		from,
	)
	if err != nil {
		return Invoice{}, err
	}
	if written.RowsAffected() == 0 {
		return Invoice{}, ErrPaymentAlreadySettled
	}
	return s.read(ctx, invoiceByIDSelection, invoiceID)
}

// paymentColumns is what one outcome writes into the three columns of a state: the moment a
// settlement is stated by, the moment a refusal is, and the reason a refusal carries. What the state
// does not carry is absent, so a row cannot end up holding a moment or a reason its own status does
// not explain.
type paymentColumns struct {
	paidAt      *time.Time
	failedAt    *time.Time
	failureCode *FailureCode
}

func outcomeColumns(outcome SettleOutcome) paymentColumns {
	switch outcome.Status {
	case PaidPayment:
		moment := outcome.Moment
		return paymentColumns{paidAt: &moment}
	case FailedPayment:
		moment := outcome.Moment
		code := outcome.FailureCode
		return paymentColumns{failedAt: &moment, failureCode: &code}
	default:
		return paymentColumns{}
	}
}

// read reads at most one invoice, so that a selection matching several rows is reported as a failure
// of the caller's expectation rather than silently answering the first.
func (s *Store) read(ctx context.Context, selection string, arguments ...any) (Invoice, error) {
	return database.ReadOne(ctx, database.QuerierFrom(ctx, s.pool),
		scanInvoice, ErrInvoiceNotFound, selection, arguments...)
}
