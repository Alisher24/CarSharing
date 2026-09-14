package invoices

import (
	"errors"
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
)

// PaymentStatus is what is owed on an invoice. Every status the contract publishes is declared here,
// because the storage admits all three and the read of one has to name what it read; the transitions
// between them belong to the task that owns payment.
type PaymentStatus string

const (
	// PendingPayment is an invoice nothing has settled yet. A positive invoice this build issues
	// states it and waits for the attempt the service owes it.
	PendingPayment PaymentStatus = "pending"

	// FailedPayment is an attempt that was refused. The invoice is unchanged and may be attempted
	// again.
	FailedPayment PaymentStatus = "failed"

	// PaidPayment is an invoice that is settled.
	PaidPayment PaymentStatus = "paid"
)

// Known reports whether a status is one this vocabulary declares, which is what a row read from
// storage is judged by before it is published under a shape that states one.
func (s PaymentStatus) Known() bool {
	return s == PendingPayment || s == FailedPayment || s == PaidPayment
}

// FailureCode is why an attempt at an invoice was refused. The contract publishes exactly one of
// these, and the two states that carry it are the same statement: a refusal without a code and a code
// without a refusal are both defects of the row that holds them.
type FailureCode string

// Declined is the one reason the contract publishes for a refused payment: the attempt was turned
// down. The code is stated once here, and the check constraint of the table admits the same word.
const Declined FailureCode = "declined"

// DeclinedFailureCode is that reason as the contract spells it on the wire, so the rule that decides a
// refusal states the word the contract declares rather than a copy of it.
const DeclinedFailureCode = string(Declined)

// Known reports whether a reason is one this vocabulary declares, which is what a row read from
// storage is judged by before it is published under a shape that states one.
func (c FailureCode) Known() bool { return c == Declined }

// SettleOutcome is what one payment transition reaches: the state the payment takes, the moment that
// state is stated by, and — for a refusal — the reason the contract publishes for it. One shape
// answers every transition, so a settlement and a refusal are described by one value rather than by
// two calls with different arguments.
type SettleOutcome struct {
	Status PaymentStatus
	Moment time.Time

	// FailureCode is why an attempt was refused. A status that is not a refusal carries no code, and
	// the transition refuses to write a refusal without one.
	FailureCode FailureCode
}

// Paid is the outcome of an attempt that settled the invoice.
func Paid(moment time.Time) SettleOutcome {
	return SettleOutcome{Status: PaidPayment, Moment: moment}
}

// Refused is the outcome of an attempt that was turned down for the given reason.
func Refused(moment time.Time, code FailureCode) SettleOutcome {
	return SettleOutcome{Status: FailedPayment, Moment: moment, FailureCode: code}
}

// Validate reports why an outcome cannot be one a payment transition writes, or nil when it can. The
// storage states the same rules again, so an outcome that passed here and was refused there is a
// defect rather than the only check there was.
func (o SettleOutcome) Validate() error {
	switch o.Status {
	case PaidPayment, FailedPayment:
	default:
		return fmt.Errorf("a payment attempt cannot end in the state %q", o.Status)
	}
	if o.Moment.IsZero() {
		return errors.New("a payment attempt must state the moment it ended")
	}
	if o.Status == FailedPayment && !o.FailureCode.Known() {
		return fmt.Errorf("a refused attempt cannot state the reason %q", o.FailureCode)
	}
	if o.Status == PaidPayment && o.FailureCode != "" {
		return errors.New("a settled payment cannot carry a reason for a refusal")
	}
	return nil
}

// FirstPayment is the state of the payment an invoice is issued with. A ride that cost nothing owes
// nothing, so its invoice is settled by the moment it was issued and no attempt is owed; one that cost
// something waits for the attempt the service makes. Every reader asks this function rather than
// comparing an amount with zero a second time, so the two answers cannot disagree.
func FirstPayment(total billing.AmountTyiyn, issuedAt time.Time) (PaymentStatus, *time.Time) {
	if total == 0 {
		settled := issuedAt
		return PaidPayment, &settled
	}
	return PendingPayment, nil
}

// ErrPaymentAlreadySettled reports a transition the payment of an invoice has already made: the state
// the transition starts from is not the state the row stands in. A second attempt at a settled invoice
// meets it, and so does a repeat of the attempt that stored a refusal.
var ErrPaymentAlreadySettled = errors.New("the payment of this invoice is not in the state its transition starts from")
