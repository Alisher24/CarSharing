package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
)

// Every state of a payment the contract publishes is published with the moments that state carries and
// no others: one that is still being attempted states when its view last moved, a refused one states
// when it was refused and why, and a settled one states when it was paid. The moment of another state
// is not borrowed, because a client reading a settlement date off a refused payment would be reading a
// fact nobody wrote.
func TestEachPaymentStateIsPublishedWithItsOwnMoments(t *testing.T) {
	moment := rideMoment()
	declined := invoices.Declined
	for _, asked := range []struct {
		name    string
		payment invoices.Invoice
		status  string
		fields  []string
	}{
		{
			name: "an invoice the service still owes an attempt on",
			payment: invoices.Invoice{
				Payment:          invoices.PendingPayment,
				PaymentVersion:   1,
				PaymentUpdatedAt: moment,
			},
			status: "pending",
			fields: []string{"status", "updated_at"},
		},
		{
			name: "an attempt that was refused",
			payment: invoices.Invoice{
				Payment:          invoices.FailedPayment,
				PaymentVersion:   2,
				PaymentUpdatedAt: moment,
				FailedAt:         &moment,
				FailureCode:      &declined,
			},
			status: "failed",
			fields: []string{"status", "failed_at", "failure_code"},
		},
		{
			name: "an invoice that is settled",
			payment: invoices.Invoice{
				Payment:          invoices.PaidPayment,
				PaymentVersion:   3,
				PaymentUpdatedAt: moment,
				PaidAt:           &moment,
			},
			status: "paid",
			fields: []string{"status", "paid_at"},
		},
	} {
		t.Run(asked.name, func(t *testing.T) {
			published, err := paymentBody(asked.payment)
			if err != nil {
				t.Fatalf("the payment was refused: %v", err)
			}
			encoded, err := json.Marshal(published)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err = json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if status, _ := fields["status"].(string); status != asked.status {
				t.Errorf("the payment is published as status %q, want %q", status, asked.status)
			}
			if len(fields) != len(asked.fields) {
				t.Fatalf("the payment carries %d fields, want %d: %v", len(fields), len(asked.fields), fields)
			}
			for _, field := range asked.fields {
				if _, carried := fields[field]; !carried {
					t.Errorf("the payment does not carry %q", field)
				}
			}
		})
	}
}

// A state this build does not produce is a defect of the server: publishing it under another state's
// shape would tell a client that an invoice was paid when nothing settled it, and publishing a
// settlement without the moment it happened would date a payment nobody made.
func TestAPaymentThisBuildCannotPublishIsReported(t *testing.T) {
	moment := rideMoment()
	declined := invoices.Declined
	for _, refused := range []struct {
		name    string
		payment invoices.Invoice
	}{
		{name: "a state the contract does not declare", payment: invoices.Invoice{
			Payment: invoices.PaymentStatus("refunded"), PaymentUpdatedAt: moment,
		}},
		{name: "a refusal without a moment", payment: invoices.Invoice{
			Payment: invoices.FailedPayment, PaymentUpdatedAt: moment, FailureCode: &declined,
		}},
		{name: "a refusal without a reason", payment: invoices.Invoice{
			Payment: invoices.FailedPayment, PaymentUpdatedAt: moment, FailedAt: &moment,
		}},
		{name: "a refusal with a reason nobody publishes", payment: invoices.Invoice{
			Payment: invoices.FailedPayment, PaymentUpdatedAt: moment, FailedAt: &moment,
			FailureCode: failureCode("expired"),
		}},
		{name: "a settlement without a moment", payment: invoices.Invoice{
			Payment: invoices.PaidPayment, PaymentUpdatedAt: moment,
		}},
	} {
		t.Run(refused.name, func(t *testing.T) {
			if _, err := paymentBody(refused.payment); err == nil {
				t.Fatal("the payment was published")
			}
		})
	}
}

// failureCode names a reason the contract does not declare, so a check can present one.
func failureCode(code string) *invoices.FailureCode {
	reason := invoices.FailureCode(code)
	return &reason
}

// The payment operation answers its own shape at every status it states, so a replayed refusal comes
// back as the operation that was asked rather than as one of its siblings, and a status it does not
// declare is reported instead of written.
func TestThePaymentOperationDeclaresEachAnswerItGives(t *testing.T) {
	operation := payInvoiceOperation
	for _, status := range []int{200, 404, 409, 503} {
		if _, declared := operation.answers[status]; !declared {
			t.Errorf("the payment operation answers %d without a shape", status)
		}
	}
	if operation.refused != 409 {
		t.Errorf("the payment operation refuses with %d, want 409", operation.refused)
	}
	if operation.route("inv-1") != "/api/v1/me/invoices/inv-1/pay" {
		t.Errorf("the payment fingerprint covers %q", operation.route("inv-1"))
	}
}

// The wait a refusal asks for is the one a payment in flight carries, and nothing else borrows it: an
// exhausted allowance states the moment it returns in its own body rather than in this header.
func TestOnlyTheAnswersThatAskForAWaitCarryOne(t *testing.T) {
	for _, asked := range []struct {
		code  servedapi.ErrorCode
		waits bool
	}{
		{code: servedapi.PAYMENTINPROGRESS, waits: true},
		{code: servedapi.IDEMPOTENCYINPROGRESS, waits: true},
		{code: servedapi.OUTSTANDINGINVOICE},
		{code: servedapi.DAILYLIMITREACHED},
		{code: servedapi.RESOURCENOTFOUND},
	} {
		retryAfter := retryAfterOf(servedapi.ApiError{Code: asked.code})
		if (retryAfter != nil) != asked.waits {
			t.Errorf("the %s answer asks for a wait: %v, want %v",
				asked.code, retryAfter != nil, asked.waits)
		}
		if retryAfter != nil && *retryAfter != 1 {
			t.Errorf("the %s answer asks to wait %d seconds, want 1", asked.code, *retryAfter)
		}
	}
}

// A moment of a payment is published in the one form the contract states, so the same instant read
// from storage and written into an answer is the same string a client parses.
func TestAPaymentMomentIsPublishedAsTheContractStatesIt(t *testing.T) {
	moment := time.Date(2026, time.September, 14, 7, 30, 30, 123456000, time.UTC)
	published, err := paymentBody(invoices.Invoice{
		Payment:          invoices.PaidPayment,
		PaymentUpdatedAt: moment,
		PaidAt:           &moment,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(published)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err = json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["paid_at"] != "2026-09-14T07:30:30.123456Z" {
		t.Fatalf("the moment is published as %q", fields["paid_at"])
	}
}
