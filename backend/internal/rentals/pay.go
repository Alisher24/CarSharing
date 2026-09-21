package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PayCommand asks to pay one invoice of one account, whoever asks. Nothing about the payment travels
// with it: the contract states that the outcome of a test payment is decided by the service, so no
// amount, no result and no owner is taken from the request.
type PayCommand struct {
	Caller    uuid.UUID
	InvoiceID string
	Attempt   Attempt
}

// Pay settles one invoice of the caller, or answers why it cannot.
//
// The command is the same shape as the ending of a ride: one transaction under the shared lock order,
// the state read again under those locks, and the transition written by the module that owns it. The
// account and the ride are what it locks — the invoice itself is named by the ride — and the decision
// is taken from the payment as it stands under those locks rather than from what the caller expected.
func (s *Service) Pay(ctx context.Context, command PayCommand) (Answered, error) {
	return s.answer(ctx, idempotency.ForAccount(command.Caller), command.Attempt,
		payParticipants(s.pool, command),
		func(ctx context.Context, _ pgx.Tx, moment time.Time) (Outcome, error) {
			return s.payWithin(ctx, moment, command)
		})
}

// payParticipants is the rows a payment touches: the account that owes the invoice and the ride it
// describes. Which two they are is read from the invoice, and the invoice itself is left out of the
// plan: it belongs to a ride, so locking the ride is what keeps its payment from moving under this
// command.
func payParticipants(pool *pgxpool.Pool, command PayCommand) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		issued, rentalID, err := invoices.NewStore(pool).ByIDForRead(ctx, command.InvoiceID)
		if errors.Is(err, invoices.ErrInvoiceNotFound) {
			// An invoice no account holds is answered the same way whether it does not exist or
			// belongs to somebody else, so nothing is locked for it.
			return participants{}, nil
		}
		if err != nil {
			return participants{}, err
		}
		return participants{
			users:   []uuid.UUID{issued.UserID},
			rentals: sortedIdentifiers(rentalID),
		}, nil
	}
}

// payWithin decides one payment with the participants locked and the moment fixed.
func (s *Service) payWithin(ctx context.Context, moment time.Time, command PayCommand) (Outcome, error) {
	issued, _, err := s.invoices.ByIDForRead(ctx, command.InvoiceID)
	if errors.Is(err, invoices.ErrInvoiceNotFound) {
		return refused(moment, Refusal{Kind: InvoiceNotFound}), nil
	}
	if err != nil {
		return Outcome{}, err
	}
	// Another account's invoice is its own business: the answer is the same one a name nobody holds
	// gets, so it cannot be used to learn that the invoice exists.
	if issued.UserID != command.Caller {
		return refused(moment, Refusal{Kind: InvoiceNotFound}), nil
	}

	action, refusal := ManualPayment(PaymentStage(issued.Payment))
	if refusal != "" {
		return refused(moment, Refusal{Kind: refusal}), nil
	}
	if action == LeaveInvoice {
		return Outcome{Moment: moment, Invoice: issued}, nil
	}

	// A manual attempt is a new key by definition — the key of the attempt that was refused
	// reproduces its answer instead — so the transition it makes is the one a refusal permits.
	settled, err := s.attemptPayment(ctx, issued, moment)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Moment: moment, Invoice: settled}, nil
}

// attemptPayment makes the attempt a person asked for and records the signal of the change it made.
// Nothing here reads a demand: a demand decides the attempt the service owes, and the attempt a person
// asks for is the person's own.
func (s *Service) attemptPayment(
	ctx context.Context, issued invoices.Invoice, moment time.Time,
) (invoices.Invoice, error) {
	settled, err := s.invoices.Settle(ctx, issued.ID, invoices.FailedPayment, invoices.Paid(moment))
	if err != nil {
		return invoices.Invoice{}, err
	}
	if err = events.Record(ctx, s.pool, events.Signal{
		Kind:       events.InvoiceChanged,
		ResourceID: settled.ID,
		Version:    settled.PaymentVersion,
		Recipient:  settled.UserID,
	}); err != nil {
		return invoices.Invoice{}, err
	}
	return settled, nil
}
