package rentals

import "github.com/Alisher24/CarSharing/backend/internal/invoices"

// The payment of an invoice is decided by rules that read nothing: which of them applies follows from
// the state the payment of an invoice stands in and from the demand a demonstration may have recorded
// for the attempt, and both arrive as arguments. Nothing here reads the database, waits, or writes, so
// the same two answers serve the command a person sends and the attempt a worker delivers.
//
// The states are the three the contract publishes. They are named again rather than taken from the
// module that stores them, because what this file decides is which of them permits which command, and
// a rule that spoke the storage's words would answer with the storage's rows.

// PaymentStage is the state the payment of an invoice stands in, as the rules of this module read it.
type PaymentStage string

const (
	// PaymentPending is an invoice the service has not attempted yet. The first attempt is the service's
	// own work, so a person waits for it rather than paying instead of it.
	PaymentPending PaymentStage = "pending"

	// PaymentFailed is an attempt that was refused. The invoice is unchanged, and a person may ask for
	// another attempt with a key of its own.
	PaymentFailed PaymentStage = "failed"

	// PaymentPaid is an invoice nothing is owed on.
	PaymentPaid PaymentStage = "paid"
)

// ManualAction is what a command a person sends does with the invoice it names.
type ManualAction string

const (
	// LeaveInvoice is an invoice that is already settled: the answer is the view that exists, and no
	// attempt is made, no moment moves and no version grows.
	LeaveInvoice ManualAction = "leave"

	// AttemptPayment is one attempt at an unsettled invoice.
	AttemptPayment ManualAction = "attempt"
)

// manualPaymentRule is the whole of what a manual command does in one state of a payment: what it
// does, and the refusal it answers with when it does nothing. Stating both together is what keeps a
// state from permitting an attempt and refusing it at the same time.
type manualPaymentRule struct {
	action  ManualAction
	refusal RefusalKind
}

// manualPaymentRules is what a command a person sends does in each state. Only a refusal permits an
// attempt: while the service still owes the first attempt there is nothing for a person to repeat, and
// a settled invoice has nothing left to do — which is what makes a new key on a paid invoice answer the
// view that exists rather than a second payment.
var manualPaymentRules = map[PaymentStage]manualPaymentRule{
	PaymentPaid:   {action: LeaveInvoice},
	PaymentFailed: {action: AttemptPayment},
	PaymentPending: {
		action:  AttemptPayment,
		refusal: PaymentInProgress,
	},
}

// ManualPayment reports what a command a person sent does with an invoice whose payment stands in the
// given state, and which refusal it answers with when it does nothing. A state this vocabulary does not
// declare permits nothing and is refused as a payment that is still running: an invoice whose state
// nobody can explain must not be paid a second time.
func ManualPayment(stage PaymentStage) (ManualAction, RefusalKind) {
	rule, known := manualPaymentRules[stage]
	if !known {
		return LeaveInvoice, PaymentInProgress
	}
	return rule.action, rule.refusal
}

// DemoDemand is the recorded demand for the outcome of the next attempt at one ride: what was asked
// for, and whether anything was asked at all. The demand is read from the table that holds it and
// handed here, because a rule of this file reads nothing itself.
type DemoDemand struct {
	Outcome invoices.DemoOutcome
	Asked   bool
}

// AttemptOutcome is how one attempt at an invoice ends: the state it reaches, the reason a refusal
// carries, and whether the attempt spent the demand that decided it.
type AttemptOutcome struct {
	Status PaymentStage

	// FailureCode is why the attempt was refused, which the contract publishes as one word.
	FailureCode string

	// Spent reports whether this attempt used up a recorded demand. A demand is asked for once and
	// settles one attempt, so the attempt that read it is the one that removes it, in the transaction
	// that read it.
	Spent bool
}

// OutcomeOf decides how one attempt at an invoice ends. A demand for a refusal ends it as the one
// refusal the contract publishes; without a demand, or with one asking for success, the attempt
// succeeds. Either way the attempt spends the demand it found, so setting a demand back to success
// cancels an earlier refusal rather than leaving it for the attempt after this one.
func OutcomeOf(demand DemoDemand) AttemptOutcome {
	if demand.Asked && demand.Outcome == invoices.DemoFailed {
		return AttemptOutcome{
			Status:      PaymentFailed,
			FailureCode: invoices.DeclinedFailureCode,
			Spent:       true,
		}
	}
	return AttemptOutcome{Status: PaymentPaid, Spent: demand.Asked}
}
