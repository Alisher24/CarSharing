package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/google/uuid"
)

// InvoiceReads is what reading the caller's own invoices needs: one of them by its identifier, and
// one page of them newest first. Each answers the immutable invoice together with the state of its
// payment, and an invoice this account does not hold is reported as absent.
type InvoiceReads interface {
	ByID(ctx context.Context, owner uuid.UUID, id string) (invoices.Invoice, error)
	ReadPage(ctx context.Context, owner uuid.UUID, after *cursor.Position, limit int) (invoices.Page, error)
}

// invoiceHandlers answers the reads of the caller's own invoices: one of them, and the collection
// they are listed in.
//
// An invoice is read by an account that did not necessarily cause it: a ride the service ended because
// its sources ran out produces an invoice nobody asked for, and the account that rode it opens the
// result from the report of the ending. That is why this read exists, and why it answers the invoice
// together with the state of its payment: a payment that moved after the result was first shown must
// be readable again rather than remembered.
type invoiceHandlers struct {
	invoices InvoiceReads
	cursors  *cursor.Signer
}

func newInvoiceHandlers(reads InvoiceReads, cursors *cursor.Signer) (invoiceHandlers, error) {
	if reads == nil {
		return invoiceHandlers{}, fmt.Errorf("%w: invoice reads", ErrIncompleteApplication)
	}
	if cursors == nil {
		return invoiceHandlers{}, fmt.Errorf("%w: cursor signer", ErrIncompleteApplication)
	}
	return invoiceHandlers{invoices: reads, cursors: cursors}, nil
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

func registerInvoiceHandlers(served *server, dependencies Dependencies) error {
	var err error
	served.invoiceHandlers, err = newInvoiceHandlers(dependencies.Invoices, dependencies.Cursors)
	return err
}
