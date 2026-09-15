package httpapi

import (
	"context"
	"log/slog"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/google/uuid"
)

// getInvoicesOperation is the name the signed cursors of this collection are bound to: the
// operation's own identifier in the contract.
const getInvoicesOperation = "getInvoices"

// GetInvoices answers one page of the caller's own invoices, newest first, each with the state of
// its payment. The owner comes from the session and never from a parameter, so another account's
// invoices cannot be asked for.
//
// The collection is ordered by the moment each invoice was issued, which is not the order the ride
// history is in: the two are read as two collections rather than stitched together, because a
// position in one says nothing about a position in the other.
func (h invoiceHandlers) GetInvoices(
	ctx context.Context, request servedapi.GetInvoicesRequestObject,
) (servedapi.GetInvoicesResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.GetInvoices401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	limit := pageLimitOf(request.Params.Limit, invoices.PageSize)

	after, err := h.positionOf(request.Params.Cursor, caller, limit)
	if err != nil {
		return servedapi.GetInvoices400JSONResponse{
			Body: apiErrorBody(ctx, codeInvalidCursor, messageInvalidCursor),
		}, nil
	}
	page, err := h.invoices.ReadPage(ctx, caller, after, limit)
	if err != nil {
		slog.ErrorContext(ctx, "the invoice collection could not be read", "error", err)
		return servedapi.GetInvoices503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	body, err := h.collectionBody(page, caller, limit)
	if err != nil {
		return nil, err
	}
	return servedapi.GetInvoices200JSONResponse{Body: body}, nil
}

// positionOf reads the position a presented cursor names, in the vocabulary of the module that pages
// the collection.
func (h invoiceHandlers) positionOf(
	presented *servedapi.Cursor, owner uuid.UUID, limit int,
) (*invoices.Position, error) {
	read, err := pagePositionOf(h.cursors, presented, accountPageScope(getInvoicesOperation, owner, limit))
	if err != nil || read == nil {
		return nil, err
	}
	return &invoices.Position{IssuedAt: read.CreatedAt, ID: read.ID}, nil
}

// collectionBody renders one page of the collection, with the cursor that reads the page after it. A
// last or empty page carries no cursor: there is nothing to continue from, and the contract
// publishes that as a null cursor.
func (h invoiceHandlers) collectionBody(
	page invoices.Page, owner uuid.UUID, limit int,
) (servedapi.InvoiceCollection, error) {
	items := make([]servedapi.InvoiceView, 0, len(page.Invoices))
	for _, issued := range page.Invoices {
		item, err := invoiceViewBody(issued)
		if err != nil {
			return servedapi.InvoiceCollection{}, err
		}
		items = append(items, item)
	}
	body := servedapi.InvoiceCollection{Items: items}
	if page.Next == nil {
		return body, nil
	}
	issued, err := h.cursors.Issue(cursor.Position{
		CreatedAt: page.Next.IssuedAt,
		ID:        page.Next.ID,
	}, accountPageScope(getInvoicesOperation, owner, limit))
	if err != nil {
		return servedapi.InvoiceCollection{}, err
	}
	body.NextCursor = &issued
	return body, nil
}
