package httpapi

import (
	"context"
	"fmt"
	"net/http"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// payInvoicePath is the route of the command that pays an invoice. It is the specification's own path,
// stated once here so that the handler and the fingerprint of the command name the same one.
const payInvoicePath = "/api/v1/me/invoices/{id}/pay"

// payHandlers answers the command that pays an invoice. It is the same shape as the ending of a ride:
// the caller is resolved, the attempt of the operation is built, and the answer is spelled the one way
// a command answer is.
type payHandlers struct{ reservations Reservations }

func newPayHandlers(reservations Reservations) (payHandlers, error) {
	if reservations == nil {
		return payHandlers{}, fmt.Errorf("%w: paying an invoice", ErrIncompleteApplication)
	}
	return payHandlers{reservations: reservations}, nil
}

// PayInvoice pays the caller's invoice and answers with the state it reached.
func (h payHandlers) PayInvoice(
	ctx context.Context, request servedapi.PayInvoiceRequestObject,
) (servedapi.PayInvoiceResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.PayInvoice401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	attempt, err := payInvoiceOperation.attemptOf(string(request.Id),
		commandKeyHeader(request.Params.IdempotencyKey), payRender(ctx))
	if err != nil {
		return nil, err
	}
	answered, err := h.reservations.PayInvoice(ctx, rentals.PayCommand{
		Caller:    caller,
		InvoiceID: string(request.Id),
		Attempt:   attempt,
	})
	spelled, err := payAnswer(ctx, answered, err)
	if err != nil {
		return nil, err
	}
	return spelled.(servedapi.PayInvoiceResponseObject), nil
}

// payAnswer spells a payment that was decided, or the failure that kept it from being decided: the
// invoice with the state of its payment, the refusal the module stored for this key, or an answer of
// this server.
func payAnswer(ctx context.Context, answered rentals.Answered, failure error) (any, error) {
	if failure == nil {
		return answerOf(payInvoiceOperation, answered)
	}
	reported := commandFailureOf(failure)
	reportUncarried(ctx, failure, reported)
	return spellFailure(payInvoiceOperation, ctx, reported)
}

// payRender spells what a payment decided: the invoice with the state of its payment, and the refusal
// it decided on instead.
func payRender(ctx context.Context) rentals.Render {
	return func(outcome rentals.Outcome) (rentals.Response, error) {
		if outcome.Refused() {
			return refusalRender(ctx, payInvoiceOperation, outcome.Refusal)
		}
		body, err := paidBody(outcome)
		if err != nil {
			return rentals.Response{}, err
		}
		return encoded(http.StatusOK, body)
	}
}

func registerPayHandlers(served *server, dependencies Dependencies) error {
	var err error
	served.payHandlers, err = newPayHandlers(dependencies.Reservations)
	return err
}
