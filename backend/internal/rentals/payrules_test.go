package rentals

import (
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/invoices"
)

// What a person's command does is decided by the state the payment of the invoice stands in. A settled
// invoice answers the view that exists without another attempt, a refused one permits a new attempt,
// and one whose first attempt the service still owes permits nothing: the service is already paying it,
// and a person repeating it would be paying instead of the service rather than after it.
func TestWhatAManualCommandDoesFollowsFromThePaymentState(t *testing.T) {
	for _, asked := range []struct {
		name    string
		stage   PaymentStage
		action  ManualAction
		refusal RefusalKind
	}{
		{
			name:   "a settled invoice is left as it is",
			stage:  PaymentPaid,
			action: LeaveInvoice,
		},
		{
			name:   "a refused invoice is attempted again",
			stage:  PaymentFailed,
			action: AttemptPayment,
		},
		{
			name:    "an invoice the service still owes an attempt on is refused",
			stage:   PaymentPending,
			action:  AttemptPayment,
			refusal: PaymentInProgress,
		},
		{
			name:    "a state nobody declares is refused rather than paid again",
			stage:   PaymentStage("refunded"),
			action:  LeaveInvoice,
			refusal: PaymentInProgress,
		},
	} {
		t.Run(asked.name, func(t *testing.T) {
			action, refusal := ManualPayment(asked.stage)
			if action != asked.action {
				t.Errorf("the command %s, want %s", action, asked.action)
			}
			if refusal != asked.refusal {
				t.Errorf("the command refuses with %q, want %q", refusal, asked.refusal)
			}
		})
	}
}

// Every state the storage admits is one these rules answer for. A state that reached the command
// without a rule would be refused as a payment in flight, which is the safe answer but not the one the
// state deserves, so the two vocabularies are held together here.
func TestEveryPaymentStateHasAManualRule(t *testing.T) {
	for _, stage := range []PaymentStage{PaymentPending, PaymentFailed, PaymentPaid} {
		if _, known := manualPaymentRules[stage]; !known {
			t.Errorf("the state %q has no rule", stage)
		}
	}
	for _, status := range []invoices.PaymentStatus{
		invoices.PendingPayment, invoices.FailedPayment, invoices.PaidPayment,
	} {
		if _, known := manualPaymentRules[PaymentStage(status)]; !known {
			t.Errorf("the stored state %q has no rule", status)
		}
	}
}

// One attempt is decided by the demand a demonstration recorded for it, and the demand is spent by the
// attempt it decided: none at all and one asking for success both succeed, one asking for a refusal
// ends as the one refusal the contract publishes, and every one of them is spent so that the attempt
// after this one is decided afresh.
func TestOneAttemptIsDecidedByTheDemandItSpends(t *testing.T) {
	for _, asked := range []struct {
		name   string
		demand DemoDemand
		status PaymentStage
		code   string
		spent  bool
	}{
		{
			name:   "nothing was asked for",
			demand: DemoDemand{},
			status: PaymentPaid,
		},
		{
			name:   "success was asked for",
			demand: DemoDemand{Outcome: invoices.DemoPaid, Asked: true},
			status: PaymentPaid,
			spent:  true,
		},
		{
			name:   "a refusal was asked for",
			demand: DemoDemand{Outcome: invoices.DemoFailed, Asked: true},
			status: PaymentFailed,
			code:   invoices.DeclinedFailureCode,
			spent:  true,
		},
	} {
		t.Run(asked.name, func(t *testing.T) {
			decided := OutcomeOf(asked.demand)
			if decided.Status != asked.status {
				t.Errorf("the attempt ends as %q, want %q", decided.Status, asked.status)
			}
			if decided.FailureCode != asked.code {
				t.Errorf("the refusal carries %q, want %q", decided.FailureCode, asked.code)
			}
			if decided.Spent != asked.spent {
				t.Errorf("the demand is spent: %v, want %v", decided.Spent, asked.spent)
			}
		})
	}
}

// A demand decides one attempt and one ride: the same demand asked about twice decides both attempts
// the same way only because the storage hands it to one of them, and a demand recorded for another
// ride is not a demand for this one. What the rule can state is that a demand for success cancels an
// earlier refusal rather than being read as one.
func TestADemandForSuccessCancelsAnEarlierRefusal(t *testing.T) {
	refused := OutcomeOf(DemoDemand{Outcome: invoices.DemoFailed, Asked: true})
	if refused.Status != PaymentFailed {
		t.Fatalf("the refusal was not decided: %s", refused.Status)
	}
	cancelled := OutcomeOf(DemoDemand{Outcome: invoices.DemoPaid, Asked: true})
	if cancelled.Status != PaymentPaid {
		t.Fatalf("a demand for success ends as %s, want %s", cancelled.Status, PaymentPaid)
	}
	if !cancelled.Spent {
		t.Error("a demand for success was not spent, so it would decide the next attempt as well")
	}
}
