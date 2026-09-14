package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// paymentAttemptTask names the durable work an ending owes the payment of its invoice: one attempt at
// what the ride cost. It is named here, beside the delivery that performs it, and the process that
// delivers it declares the same kind where the deliveries of the queue are assembled.
//
// The letter with the invoice is a task of its own, recorded by the same ending: this one is work the
// service does to a payment, and the two are delivered by different code.
const paymentAttemptTask events.Kind = "payment.attempt"

// PaymentAttemptTask is the kind of task the delivery below performs, for the composition root that
// names it in the table of deliveries.
func PaymentAttemptTask() string { return string(paymentAttemptTask) }

// RentalPayment delivers one attempt at the invoice of one ride: the part of a payment that a worker
// performs, told the task the queue holds.
//
// It is a type of its own rather than a method of Service because the worker builds it from the pool
// alone: the delivery takes no HTTP request, is reached under the lease of the queue rather than by a
// caller, and must be constructible by the process that delivers tasks — while Service is assembled by
// the API process, which is not the one that makes the first attempt.
type RentalPayment struct {
	pool      *pgxpool.Pool
	outcomes  *invoices.Outcomes
	invoicing *invoices.Store
}

// NewRentalPayment assembles the delivery over one connection pool. A delivery that reached a missing
// dependency would claim a task it could never settle, so the failure is here rather than at the first
// attempt.
func NewRentalPayment(pool *pgxpool.Pool) (*RentalPayment, error) {
	if pool == nil {
		return nil, errors.New("the payment attempt delivery is missing a database pool")
	}
	return &RentalPayment{
		pool:      pool,
		outcomes:  invoices.NewOutcomes(pool),
		invoicing: invoices.NewStore(pool),
	}, nil
}

// Attempt makes the first attempt at the invoice the task names, or reports that there was nothing
// left to attempt.
//
// The whole attempt is one transaction under the shared lock order: the account that owes the invoice
// is locked, then the ride, the relationships are read again under those locks, and only then are the
// moment and the outcome of the attempt fixed. A second delivery of the same task — an expired lease,
// a restarted worker, a second copy of it — finds a payment that is no longer waiting and writes
// nothing, which is what makes one invoice settle once.
func (p *RentalPayment) Attempt(ctx context.Context, invoiceID string) (PaymentResult, error) {
	var result PaymentResult
	err := transact(ctx, p.pool,
		paymentParticipants(p.pool, invoiceID),
		func(txCtx context.Context, moment time.Time) error {
			attempted, err := p.attemptWithin(txCtx, moment, invoiceID)
			if err != nil {
				return err
			}
			result = attempted
			return nil
		})
	if err != nil {
		return PaymentResult{}, err
	}
	return result, nil
}

// PaymentResult is what one attempt at an invoice did: the invoice as it now stands, and whether a
// transition happened at all. An invoice that was already settled — or whose ride is gone — is
// reported as untouched rather than as a payment, because a delivery that wrote nothing must not be
// reported as one that paid.
type PaymentResult struct {
	Invoice invoices.Invoice
	Settled bool
}

// paymentParticipants is the rows one attempt at an invoice touches: the account that owes it and the
// ride it describes. The vehicle is not among them, because nothing about paying reads the catalog,
// and taking a row another transition is waiting for would make an attempt wait for a ride it does not
// depend on.
func paymentParticipants(
	pool *pgxpool.Pool, invoiceID string,
) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		issued, _, err := invoices.NewStore(pool).ByIDForRead(ctx, invoiceID)
		if errors.Is(err, invoices.ErrInvoiceNotFound) {
			return participants{}, nil
		}
		if err != nil {
			return participants{}, err
		}
		return participants{
			users:   []uuid.UUID{issued.UserID},
			rentals: sortedIdentifiers(issued.RentalID),
		}, nil
	}
}

// attemptWithin decides one attempt with the participants locked and the moment fixed.
func (p *RentalPayment) attemptWithin(
	ctx context.Context, moment time.Time, invoiceID string,
) (PaymentResult, error) {
	issued, rentalID, err := p.invoicing.ByIDForRead(ctx, invoiceID)
	if errors.Is(err, invoices.ErrInvoiceNotFound) {
		// The invoice this task names is gone. Nothing is written: a task about a row that no longer
		// exists has nothing to settle, and reporting it as delivered is the honest answer.
		return PaymentResult{}, nil
	}
	if err != nil {
		return PaymentResult{}, err
	}
	if issued.Payment != invoices.PendingPayment {
		return PaymentResult{Invoice: issued}, nil
	}

	demand, err := p.demandFor(ctx, rentalID)
	if err != nil {
		return PaymentResult{}, err
	}
	decided := OutcomeOf(demand)
	settled, err := p.settle(ctx, issued, decided, moment)
	if err != nil {
		return PaymentResult{}, err
	}
	return PaymentResult{Invoice: settled, Settled: true}, nil
}

// demandFor reads the demand a demonstration recorded for the ride of this invoice and spends it. The
// read is the deletion: an attempt that is decided by a demand is the attempt that removes it, in the
// transaction that makes the decision, so a rollback leaves the demand for the attempt that follows.
func (p *RentalPayment) demandFor(ctx context.Context, rentalID string) (DemoDemand, error) {
	outcome, asked, err := p.outcomes.Consume(ctx, rentalID)
	if err != nil {
		return DemoDemand{}, err
	}
	return DemoDemand{Outcome: outcome, Asked: asked}, nil
}

// settle writes the transition the attempt reached and records the signal of the change it made. The
// invoice is the only resource the change concerns: paying it moves no ride and no vehicle, so nothing
// of either is announced.
func (p *RentalPayment) settle(
	ctx context.Context, issued invoices.Invoice, decided AttemptOutcome, moment time.Time,
) (invoices.Invoice, error) {
	outcome, err := settleOutcomeOf(decided, moment)
	if err != nil {
		return invoices.Invoice{}, err
	}
	settled, err := p.invoicing.Settle(ctx, issued.ID, invoices.PendingPayment, outcome)
	if err != nil {
		return invoices.Invoice{}, err
	}
	if err = events.Record(ctx, p.pool, events.Signal{
		Kind:       events.InvoiceChanged,
		ResourceID: settled.ID,
		Version:    settled.PaymentVersion,
		Recipient:  settled.UserID,
	}); err != nil {
		return invoices.Invoice{}, err
	}
	return settled, nil
}

// settleOutcomeOf spells the decision of the rule as the transition the storage performs. A decision
// this build cannot write is reported as a defect of the rule rather than stored as a state nobody
// asked for.
func settleOutcomeOf(decided AttemptOutcome, moment time.Time) (invoices.SettleOutcome, error) {
	if decided.Status == PaymentFailed {
		return invoices.Refused(moment, invoices.FailureCode(decided.FailureCode)), nil
	}
	if decided.Status == PaymentPaid {
		return invoices.Paid(moment), nil
	}
	return invoices.SettleOutcome{}, errors.New("the payment rule decided a state no attempt can reach")
}
