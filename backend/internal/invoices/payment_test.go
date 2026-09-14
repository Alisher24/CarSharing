package invoices

import (
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
)

// The first payment of an invoice is stated once, next to the draft it is written from: a ride that
// cost nothing owes nothing and is settled by the moment its invoice was issued, and one that cost
// something waits for the attempt the service owes it. Both answers are read from the same function,
// so the state a draft is written with and the decision whether an attempt is owed cannot disagree.
func TestTheFirstPaymentOfAnInvoiceIsSettledForNothingAndWaitingForMore(t *testing.T) {
	for _, asked := range []struct {
		name        string
		total       billing.AmountTyiyn
		status      PaymentStatus
		settledAt   bool
		attemptOwed bool
	}{
		{name: "a ride that cost nothing", total: 0, status: PaidPayment, settledAt: true},
		{name: "a ride that cost something", total: 2789, status: PendingPayment, attemptOwed: true},
		{name: "a ride that cost one tyiyn", total: 1, status: PendingPayment, attemptOwed: true},
	} {
		t.Run(asked.name, func(t *testing.T) {
			status, paidAt := FirstPayment(asked.total, draftedAt)
			if status != asked.status {
				t.Fatalf("the first payment is %q, want %q", status, asked.status)
			}
			if (paidAt != nil) != asked.settledAt {
				t.Fatalf("the first payment states a moment of settlement: %v", paidAt != nil)
			}
			if paidAt != nil && !paidAt.Equal(draftedAt) {
				t.Fatalf("a settled invoice was paid at %s, want the moment it was issued, %s",
					paidAt, draftedAt)
			}
			if owed := status == PendingPayment; owed != asked.attemptOwed {
				t.Fatalf("an attempt is owed: %v, want %v", owed, asked.attemptOwed)
			}
		})
	}
}

// The state a transition reaches is either a settlement or a refusal, and a refusal carries the one
// reason the contract publishes. Anything else — a state no transition writes, a moment nobody read, a
// reason the contract does not declare, and a reason beside a settlement — is refused before a row is
// touched.
func TestOnlyTheOutcomesATransitionCanWriteAreAccepted(t *testing.T) {
	moment := draftedAt
	for _, asked := range []struct {
		name    string
		outcome SettleOutcome
		ok      bool
	}{
		{name: "a settlement", outcome: Paid(moment), ok: true},
		{name: "a refusal", outcome: Refused(moment, Declined), ok: true},
		{name: "a state no transition writes", outcome: SettleOutcome{
			Status: PendingPayment, Moment: moment,
		}},
		{name: "no moment", outcome: Paid(time.Time{})},
		{name: "a reason nobody publishes", outcome: Refused(moment, FailureCode("expired"))},
		{name: "a refusal without a reason", outcome: Refused(moment, "")},
		{name: "a settlement with a reason", outcome: SettleOutcome{
			Status: PaidPayment, Moment: moment, FailureCode: Declined,
		}},
	} {
		t.Run(asked.name, func(t *testing.T) {
			err := asked.outcome.Validate()
			if asked.ok && err != nil {
				t.Fatalf("the outcome was refused: %v", err)
			}
			if !asked.ok && err == nil {
				t.Fatal("the outcome was accepted")
			}
		})
	}
}

// The reason a refusal is published under is one word, and a row carrying another one is a defect
// rather than a refusal written under somebody else's reason.
func TestOnlyThePublishedRefusalReasonIsKnown(t *testing.T) {
	if !Declined.Known() {
		t.Fatal("the published reason is not known")
	}
	for _, unknown := range []FailureCode{"", "expired", "Paid"} {
		if unknown.Known() {
			t.Errorf("the reason %q is known", unknown)
		}
	}
}
