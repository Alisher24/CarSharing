package invoices

import (
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/google/uuid"
)

// draftedAt and draftedFor are the moment and the account every draft below states, so a check that
// refuses a draft is about the field it changed rather than about one it left out.
var (
	draftedAt  = time.Date(2026, time.September, 14, 7, 30, 30, 123456000, time.UTC)
	draftedFor = uuid.MustParse("01994342-6ba7-7000-8000-00000000000b")
)

// drafted is an invoice of the M07 example: ninety seconds of driving begun as two minutes at 1234
// tyiyn and forty-five seconds of pause begun as one at 321, which is 2789 tyiyn in total. Its lines
// are read from a charge, which is the only way an invoice is ever written.
func drafted() Draft {
	priced, err := billing.Compute(
		billing.Rates{Driving: 1234, Paused: 321}, 90*time.Second, 45*time.Second)
	if err != nil {
		panic(err)
	}
	driving, paused := priced.Lines()
	return Draft{
		RentalID:   "01994342-6ba7-7000-8000-000000000001",
		UserID:     draftedFor,
		IssuedAt:   draftedAt,
		Completion: completion.UserFinished,
		Driving:    LineOf(driving),
		Paused:     LineOf(paused),
		TotalTyiyn: priced.TotalTyiyn,
	}
}

// The example of the specification is written as the two lines the contract publishes, in the order it
// publishes them, with the durations in whole microseconds and the rates the ride was priced at.
func TestTheExampleOfTheSpecificationIsADraftAnInvoiceCanBeWrittenFrom(t *testing.T) {
	if err := drafted().Validate(); err != nil {
		t.Fatalf("the example was refused: %v", err)
	}
}

// A draft that could not become a row, or that a reader could not explain, is refused before anything
// is written: a rental nobody named, an account nobody owes, no moment, a reason this build does not
// publish, an amount that is not the product of its own line, and a total that is not the sum of the
// two lines.
func TestADraftThatCannotBeAnInvoiceIsRefused(t *testing.T) {
	for _, refused := range []struct {
		name   string
		change func(*Draft)
	}{
		{name: "no ride", change: func(d *Draft) { d.RentalID = "" }},
		{name: "no account", change: func(d *Draft) { d.UserID = uuid.Nil }},
		{name: "no moment", change: func(d *Draft) { d.IssuedAt = time.Time{} }},
		{name: "a reason nobody publishes", change: func(d *Draft) {
			d.Completion = completion.Reason("driver_vanished")
		}},
		{name: "a total below its lines", change: func(d *Draft) { d.TotalTyiyn = 2788 }},
		{name: "a total above its lines", change: func(d *Draft) { d.TotalTyiyn = 2790 }},
		{name: "a negative total", change: func(d *Draft) { d.TotalTyiyn = -1 }},
		{name: "an amount that is not its own product", change: func(d *Draft) {
			d.Driving.AmountTyiyn = 2467
			d.TotalTyiyn = 2788
		}},
		{name: "a driving line that describes the pause", change: func(d *Draft) {
			d.Driving.Mode = billing.Paused
		}},
		{name: "a paused line that describes the driving", change: func(d *Draft) {
			d.Paused.Mode = billing.Driving
		}},
	} {
		t.Run(refused.name, func(t *testing.T) {
			draft := drafted()
			refused.change(&draft)
			if err := draft.Validate(); err == nil {
				t.Fatal("the draft was accepted")
			}
		})
	}
}

// The lines an invoice publishes are the ones it stores, in the order the contract fixes and in the
// unit the contract publishes: a reader adding up the two amounts of an invoice reads what the service
// wrote rather than a third computation of the same money.
func TestTheLinesOfAnInvoiceAreTheOnesItStores(t *testing.T) {
	issued := Invoice{Driving: drafted().Driving, Paused: drafted().Paused}

	driving, paused := issued.Lines()
	if driving.Mode != billing.Driving || paused.Mode != billing.Paused {
		t.Fatalf("the lines are published as %q then %q", driving.Mode, paused.Mode)
	}
	if driving.Duration != 90_000_000 || paused.Duration != 45_000_000 {
		t.Fatalf("the lines state %d and %d microseconds", driving.Duration, paused.Duration)
	}
	total := driving.AmountTyiyn + paused.AmountTyiyn
	if total != 2789 {
		t.Fatalf("the lines add up to %d tyiyn, want 2789", total)
	}
}

// Every status the contract publishes is known to this module, because the storage admits all three and
// a read of one names what it read. A status this build does not declare is not known, so a stored row
// carrying one is a defect rather than a state to write under another status's shape.
func TestOnlyTheStatusesTheContractDeclaresAreKnown(t *testing.T) {
	for _, asked := range []struct {
		status PaymentStatus
		known  bool
	}{
		{status: PendingPayment, known: true},
		{status: FailedPayment, known: true},
		{status: PaidPayment, known: true},
		{status: PaymentStatus("refunded")},
		{status: PaymentStatus("")},
	} {
		if asked.status.Known() != asked.known {
			t.Errorf("the status %q is known=%v, want %v",
				asked.status, asked.status.Known(), asked.known)
		}
	}
}
