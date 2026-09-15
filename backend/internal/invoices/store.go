package invoices

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInvoiceNotFound reports an invoice this account does not hold, whether no such invoice exists or
// it belongs to somebody else.
var ErrInvoiceNotFound = errors.New("no such invoice")

// Draft is an invoice about to be written: the ride it describes, the lines of the charge it states and
// the total of them. It carries no identifier and no version, because those belong to the row that is
// created rather than to the decision that asks for it.
type Draft struct {
	RentalID   string
	UserID     uuid.UUID
	IssuedAt   time.Time
	Completion completion.Reason

	// Exhausted is what a ride that ran out of energy states about it: the sources that were empty
	// when it did. A ride that was ended by a person ran out of nothing and carries none.
	Exhausted []fleet.SourceKind

	Driving Line
	Paused  Line

	TotalTyiyn billing.AmountTyiyn
}

// Validate reports why a draft cannot be written as an invoice, or nil when it can. It guards the
// statement below from a value no row would accept and a reader no row could explain; the storage
// states the same rules again, so a draft that passed here and was refused there is a defect rather
// than the only check there was.
func (d Draft) Validate() error {
	switch {
	case d.RentalID == "":
		return errors.New("an invoice must name the ride it describes")
	case d.UserID == uuid.Nil:
		return errors.New("an invoice must name the account that owes it")
	case d.IssuedAt.IsZero():
		return errors.New("an invoice must state when it was issued")
	case !d.Completion.Known():
		return fmt.Errorf("an invoice cannot state the completion reason %q", d.Completion)
	case d.TotalTyiyn < 0:
		return errors.New("an invoice cannot owe a negative amount")
	}
	total := billing.AmountTyiyn(0)
	for _, line := range []struct {
		name string
		mode billing.Mode
		line Line
	}{
		{"driving", billing.Driving, d.Driving},
		{"paused", billing.Paused, d.Paused},
	} {
		if line.line.Mode != line.mode {
			return fmt.Errorf("the %s line of an invoice describes %q", line.name, line.line.Mode)
		}
		if err := line.line.Validate(); err != nil {
			return err
		}
		if line.line.AmountTyiyn > math.MaxInt64-total {
			return errors.New("the total amount does not fit the signed 64-bit range")
		}
		total += line.line.AmountTyiyn
	}
	if total != d.TotalTyiyn {
		return errors.New("the total of an invoice is not the sum of its two lines")
	}
	return nil
}

// Validate reports why a line cannot be one of an invoice, or nil when it can.
func (l Line) Validate() error {
	switch {
	case l.Mode != billing.Driving && l.Mode != billing.Paused:
		return errors.New("a line must describe one of the two modes")
	case l.Duration < 0:
		return errors.New("a duration cannot be negative")
	case l.Minutes < 0:
		return errors.New("minutes begun cannot be negative")
	case l.Rate < 0:
		return errors.New("a rate cannot be negative")
	case l.AmountTyiyn < 0:
		return errors.New("an amount cannot be negative")
	}
	priced, err := billing.PricedAt(l.Rate, l.Minutes)
	if err != nil {
		return err
	}
	if priced != l.AmountTyiyn {
		return errors.New("an amount is not the rate of its line times the minutes begun in it")
	}
	return nil
}

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
		issuedVersion,
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
	_, err = querier.Exec(ctx, insertPaymentStatement, id, status, issuedVersion, draft.IssuedAt, paidAt)
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

// issuedVersion is the version an invoice and its payment are written at. The invoice never moves past
// it РІР‚вЂќ an invoice is immutable РІР‚вЂќ and the payment moves when its state changes.
const issuedVersion int64 = 1

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
	written, err := database.QuerierFrom(ctx, s.pool).Exec(ctx, settleStatement,
		invoiceID,
		outcome.Status,
		outcome.Moment,
		paidMomentOf(outcome),
		failedMomentOf(outcome),
		failureCodeOf(outcome),
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

// paidMomentOf, failedMomentOf and failureCodeOf state what one outcome writes into the three columns
// that belong to a state: a state that does not carry a value writes none, so the row cannot end up
// holding a moment or a reason that its own status does not explain.
func paidMomentOf(outcome SettleOutcome) *time.Time {
	if outcome.Status == PaidPayment {
		moment := outcome.Moment
		return &moment
	}
	return nil
}

func failedMomentOf(outcome SettleOutcome) *time.Time {
	if outcome.Status == FailedPayment {
		moment := outcome.Moment
		return &moment
	}
	return nil
}

func failureCodeOf(outcome SettleOutcome) *FailureCode {
	if outcome.Status == FailedPayment {
		code := outcome.FailureCode
		return &code
	}
	return nil
}

// read reads at most one invoice, so that a selection matching several rows is reported as a failure
// of the caller's expectation rather than silently answering the first.
func (s *Store) read(ctx context.Context, selection string, arguments ...any) (Invoice, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, selection, arguments...)
	if err != nil {
		return Invoice{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Invoice{}, err
		}
		return Invoice{}, ErrInvoiceNotFound
	}
	var found Invoice
	if err = scanInvoice(rows, &found); err != nil {
		return Invoice{}, err
	}
	if rows.Next() {
		return Invoice{}, errors.New("the selection matched more than one invoice")
	}
	return found, rows.Err()
}

// scanInvoice reads one row into an invoice. Every whole number is read into a plain int64 and carried
// into its own type afterwards: a named type of the same width is not one the driver plans a scan for,
// and a value it cannot plan for arrives as the zero value rather than as a failure РІР‚вЂќ an invoice of a
// ride that cost nothing is exactly the kind of record nobody would question.
func scanInvoice(rows pgx.Rows, found *Invoice) error {
	var (
		drivingDuration int64
		drivingMinutes  int64
		drivingRate     int64
		pausedDuration  int64
		pausedMinutes   int64
		pausedRate      int64
		total           int64
		exhausted       []string
	)
	err := rows.Scan(
		&found.ID,
		&found.RentalID,
		&found.UserID,
		&found.IssuedAt,
		&found.Currency,
		&found.BillingPolicy,
		&found.Completion,
		&exhausted,
		&found.Version,
		&drivingDuration,
		&drivingMinutes,
		&drivingRate,
		&pausedDuration,
		&pausedMinutes,
		&pausedRate,
		&total,
		&found.Payment,
		&found.PaymentVersion,
		&found.PaymentUpdatedAt,
		&found.PaidAt,
		&found.FailedAt,
		&found.FailureCode,
	)
	if err != nil {
		return err
	}
	found.Exhausted = fleet.SourceKinds(exhausted)
	// Which mode a line describes is where it was read rather than what a column states: the contract
	// fixes the driving line first and the paused one second, so the position of the line is the fact.
	found.Driving = Line{
		Mode:        billing.Driving,
		Duration:    billing.Microseconds(drivingDuration),
		Minutes:     drivingMinutes,
		Rate:        billing.RateTyiynPerStartedMinute(drivingRate),
		AmountTyiyn: billing.AmountTyiyn(drivingMinutes * drivingRate),
	}
	found.Paused = Line{
		Mode:        billing.Paused,
		Duration:    billing.Microseconds(pausedDuration),
		Minutes:     pausedMinutes,
		Rate:        billing.RateTyiynPerStartedMinute(pausedRate),
		AmountTyiyn: billing.AmountTyiyn(pausedMinutes * pausedRate),
	}
	found.TotalTyiyn = billing.AmountTyiyn(total)
	return nil
}
