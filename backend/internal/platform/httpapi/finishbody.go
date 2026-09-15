package httpapi

import (
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/completion"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

// finishedBody publishes what a finish decided: the ride as it ended and the invoice of it, each in the
// shape the contract declares. The rental is published from the facts the ending stored — the moment it
// began, the moment it ended, the reason it ended and the invoice it produced — so a client reads what
// the database holds rather than what the command asked for.
func finishedBody(outcome rentals.Outcome) (servedapi.FinishResult, error) {
	completed, err := completedRentalBody(outcome)
	if err != nil {
		return servedapi.FinishResult{}, err
	}
	invoice, err := invoiceViewBody(outcome.Invoice)
	if err != nil {
		return servedapi.FinishResult{}, err
	}
	return servedapi.FinishResult{
		ServerTime: timestamp.Format(outcome.Moment),
		Rental:     completed,
		Invoice:    invoice,
	}, nil
}

// paidBody publishes what the payment of one invoice decided: the invoice with the state of its
// payment as the transition left it. The answer is the invoice of the ride rather than the ride, so a
// client that asked to pay reads the amount it paid and the moment it was settled.
func paidBody(outcome rentals.Outcome) (servedapi.PayResult, error) {
	invoice, err := invoiceViewBody(outcome.Invoice)
	if err != nil {
		return servedapi.PayResult{}, err
	}
	return servedapi.PayResult{
		ServerTime: timestamp.Format(outcome.Moment),
		Invoice:    invoice,
	}, nil
}

// completedRentalBody publishes a ride that has ended. A rental in another stage cannot be described
// by this shape, so it is reported as a defect of the server rather than written as if it had ended.
func completedRentalBody(outcome rentals.Outcome) (servedapi.CompletedRental, error) {
	rental := outcome.Rental
	if rental.Stage != stage.Completed {
		return servedapi.CompletedRental{}, fmt.Errorf(
			"a rental in stage %q cannot be published as completed", rental.Stage)
	}
	vehicle, err := publishedVehicleBody(outcome.Vehicle, outcome.Moment)
	if err != nil {
		return servedapi.CompletedRental{}, err
	}
	reason, err := completionBody(reasonOf(rental), rental.Exhausted)
	if err != nil {
		return servedapi.CompletedRental{}, err
	}
	if rental.StartedAt == nil || rental.EndedAt == nil {
		return servedapi.CompletedRental{}, fmt.Errorf(
			"a completed rental in stage %q carries no ride moments", rental.Stage)
	}
	return servedapi.CompletedRental{
		Id:             rental.ID,
		Vehicle:        vehicle,
		Version:        exactInteger(rental.Version),
		ReservedAt:     timestamp.Format(rental.ReservedAt),
		TariffSnapshot: tariffSnapshotBody(rental),
		State:          servedapi.Completed,
		StartedAt:      timestamp.Format(*rental.StartedAt),
		CompletedAt:    timestamp.Format(*rental.EndedAt),
		InvoiceId:      outcome.Invoice.ID,
		Completion:     reason,
	}, nil
}

// reasonOf is why a rental ended. A rental that has ended carries one; reading a missing reason as the
// reason a person ended a ride would publish an ending nobody recorded.
func reasonOf(rental rentals.Rental) completion.Reason {
	if rental.CompletionReason == nil {
		return completion.Reason("")
	}
	return *rental.CompletionReason
}

// invoiceViewBody publishes an invoice with the state of its payment and the version of that view. The
// invoice itself never moves; the version moves with the payment beside it.
func invoiceViewBody(issued invoices.Invoice) (servedapi.InvoiceView, error) {
	invoice, err := invoiceBody(issued)
	if err != nil {
		return servedapi.InvoiceView{}, err
	}
	payment, err := paymentBody(issued)
	if err != nil {
		return servedapi.InvoiceView{}, err
	}
	return servedapi.InvoiceView{
		Version: exactInteger(issued.PaymentVersion),
		Invoice: invoice,
		Payment: payment,
	}, nil
}

// invoiceBody publishes the immutable invoice: what the ride did, what each mode of it cost, and the
// total of those two lines.
func invoiceBody(issued invoices.Invoice) (servedapi.Invoice, error) {
	reason, err := completionBody(issued.Completion, issued.Exhausted)
	if err != nil {
		return servedapi.Invoice{}, err
	}
	driving, paused := issued.Lines()
	return servedapi.Invoice{
		Id:               issued.ID,
		RentalId:         issued.RentalID,
		IssuedAt:         timestamp.Format(issued.IssuedAt),
		Currency:         servedapi.InvoiceCurrency(issued.Currency),
		BillingPolicy:    servedapi.InvoiceBillingPolicy(issued.BillingPolicy),
		Completion:       reason,
		Lines:            []servedapi.InvoiceLine{invoiceLineBody(driving), invoiceLineBody(paused)},
		TotalAmountTyiyn: exactInteger(int64(issued.TotalTyiyn)),
	}, nil
}

// invoiceLineBody publishes one line of an invoice: the mode it describes, how long the ride spent in
// it, the minutes begun in it, the rate they were priced at and their product.
func invoiceLineBody(line invoices.Line) servedapi.InvoiceLine {
	return servedapi.InvoiceLine{
		Mode:                      servedapi.InvoiceLineMode(line.Mode),
		DurationMicroseconds:      exactInteger(int64(line.Duration)),
		BilledStartedMinutes:      exactInteger(line.Minutes),
		RateTyiynPerStartedMinute: exactInteger(int64(line.Rate)),
		AmountTyiyn:               exactInteger(int64(line.AmountTyiyn)),
	}
}

// paymentBody publishes the state of what is owed on an invoice. Each status the contract declares
// carries exactly the moments it can have: a payment that is still being attempted states when its
// view last moved, a refused one states when it was refused and why, and a settled one states when it
// was paid. A status this build does not produce is reported as a defect of the server rather than
// written under another status's shape.
//
// A state that carries a moment the row does not hold is refused here for the same reason: a payment
// published with a moment nobody read would date a settlement that never happened.
func paymentBody(issued invoices.Invoice) (servedapi.Payment, error) {
	switch issued.Payment {
	case invoices.PendingPayment:
		var body servedapi.Payment
		err := body.FromPendingPayment(servedapi.PendingPayment{
			Status:    servedapi.Pending,
			UpdatedAt: timestamp.Format(issued.PaymentUpdatedAt),
		})
		return body, err
	case invoices.FailedPayment:
		if issued.FailedAt == nil || issued.FailureCode == nil || !issued.FailureCode.Known() {
			return servedapi.Payment{}, fmt.Errorf(
				"a refused payment of %s carries no moment or no reason", issued.ID)
		}
		var body servedapi.Payment
		err := body.FromFailedPayment(servedapi.FailedPayment{
			Status:      servedapi.Failed,
			FailedAt:    timestamp.Format(*issued.FailedAt),
			FailureCode: servedapi.Declined,
		})
		return body, err
	case invoices.PaidPayment:
		if issued.PaidAt == nil {
			return servedapi.Payment{}, fmt.Errorf(
				"a settled payment of %s carries no moment of settlement", issued.ID)
		}
		var body servedapi.Payment
		err := body.FromPaidPayment(servedapi.PaidPayment{
			Status: servedapi.Paid,
			PaidAt: timestamp.Format(*issued.PaidAt),
		})
		return body, err
	default:
		return servedapi.Payment{}, fmt.Errorf(
			"a payment in state %q cannot be published by this build", issued.Payment)
	}
}
