package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/google/uuid"
)

// InvoiceReads is what reading one invoice of the caller needs: the immutable invoice with the state
// of its payment, or an answer that this account holds no such invoice.
type InvoiceReads interface {
	ByID(ctx context.Context, owner uuid.UUID, id string) (invoices.Invoice, error)
}

// invoiceHandlers answers the read of one invoice.
//
// An invoice is read by an account that did not necessarily cause it: a ride the service ended because
// its sources ran out produces an invoice nobody asked for, and the account that rode it opens the
// result from the report of the ending. That is why this read exists, and why it answers the invoice
// together with the state of its payment: a payment that moved after the result was first shown must
// be readable again rather than remembered.
type invoiceHandlers struct{ invoices InvoiceReads }

func newInvoiceHandlers(reads InvoiceReads) (invoiceHandlers, error) {
	if reads == nil {
		return invoiceHandlers{}, fmt.Errorf("%w: invoice reads", ErrIncompleteApplication)
	}
	return invoiceHandlers{invoices: reads}, nil
}

// GetInvoice answers the invoice of one ride the caller holds.
func (h invoiceHandlers) GetInvoice(
	ctx context.Context, request servedapi.GetInvoiceRequestObject,
) (servedapi.GetInvoiceResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.GetInvoice401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}

	issued, err := h.invoices.ByID(ctx, caller, string(request.Id))
	if errors.Is(err, invoices.ErrInvoiceNotFound) {
		// An invoice of another account and one that never existed are answered alike, so the answer
		// cannot be used to learn that somebody else's invoice exists.
		return servedapi.GetInvoice404JSONResponse{
			Body: apiErrorBody(ctx, codeResourceNotFound, messageResourceNotFound),
		}, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "the invoice could not be read", "error", err)
		return servedapi.GetInvoice503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	body, err := invoiceViewBody(issued)
	if err != nil {
		return nil, err
	}
	return servedapi.GetInvoice200JSONResponse{Body: body}, nil
}
