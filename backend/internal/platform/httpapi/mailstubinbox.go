package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	mailstubapi "github.com/Alisher24/CarSharing/backend/internal/contracts/mailstubapi"
	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// getMessagesOperation is the name the signed cursors of the inbox collection are bound to. It is the
// operation's own identifier in the contract, so a cursor issued for another collection is refused by
// its signature rather than by a comparison somebody has to remember to write.
const getMessagesOperation = "getMessages"

// The failures of reading the box, named once because both surfaces of the inbox report them: a page
// that could not read the box states the same failure the collection does.
const (
	logBoxUnreadable    = "the mail box could not be read"
	logLetterUnreadable = "one letter could not be read"
)

// mailstubInboxHandlers answers the two operations of the read-only inbox: one page of the box and
// one letter of it. It holds no operation that changes anything, which is the whole of what this
// surface is.
type mailstubInboxHandlers struct {
	mailstubUnserved

	inbox   MailInbox
	cursors *cursor.Signer
}

// GetMessages answers one page of the box, newest first. The limit is the one the request states or
// the size the contract declares, and the cursor a client presents names a position within the same
// limit: a cursor issued for another page size belongs to another collection and is refused.
func (h mailstubInboxHandlers) GetMessages(
	ctx context.Context, request mailstubapi.GetMessagesRequestObject,
) (mailstubapi.GetMessagesResponseObject, error) {
	limit := cursor.PageSize
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	after, err := h.positionOf(request.Params.Cursor, limit)
	if err != nil {
		return mailstubapi.GetMessages400JSONResponse{
			Body: mailstubError(ctx, mailstubapi.INVALIDCURSOR, messageInvalidCursor),
		}, nil
	}
	read, err := h.pageOf(ctx, after, limit)
	if err != nil {
		slog.ErrorContext(ctx, logBoxUnreadable, "error", err)
		return mailstubapi.GetMessages503JSONResponse{Body: mailstubUnavailable(ctx)}, nil
	}
	body, err := h.collectionBody(read)
	if err != nil {
		return nil, err
	}
	return mailstubapi.GetMessages200JSONResponse{Body: body}, nil
}

// readPage is one page of the box together with the cursor that continues it, which is what both
// surfaces of the inbox answer with: the collection publishes the cursor as JSON, and the list of
// letters offers it as the address of the next page. The size and the order are the ones the request
// named, so a page read here and a page read by the collection are the same page.
type readPage struct {
	messages []mailstub.Message
	next     *string
}

// pageOf reads one page of the box and issues the cursor the page after it is read with. A cursor
// that cannot be issued is a defect of the key this installation signs with rather than a condition
// of the request, so it is reported rather than answered with a page that cannot be continued.
func (h mailstubInboxHandlers) pageOf(
	ctx context.Context, after *cursor.Position, limit int,
) (readPage, error) {
	page, err := h.inbox.ReadPage(ctx, after, limit)
	if err != nil {
		return readPage{}, err
	}
	read := readPage{messages: page.Messages}
	if page.Next == nil {
		return read, nil
	}
	issued, err := h.cursors.Issue(
		*page.Next,
		h.inboxScopeOf(limit))
	if err != nil {
		return readPage{}, err
	}
	read.next = &issued
	return read, nil
}

// GetMessage answers one letter of the box, text included and uninterpreted: what a person reads
// here is the letter the stub stored, not a rendering of it. A letter that is not there and one
// whose identifier belongs to nothing are the same answer, so the box cannot be asked what it holds.
func (h mailstubInboxHandlers) GetMessage(
	ctx context.Context, request mailstubapi.GetMessageRequestObject,
) (mailstubapi.GetMessageResponseObject, error) {
	stored, err := h.inbox.ByID(ctx, string(request.Id))
	if errors.Is(err, mailstub.ErrMessageNotFound) {
		return mailstubapi.GetMessage404JSONResponse{
			Body: mailstubError(ctx, mailstubapi.RESOURCENOTFOUND, messageResourceNotFound),
		}, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, logLetterUnreadable, "error", err)
		return mailstubapi.GetMessage503JSONResponse{Body: mailstubUnavailable(ctx)}, nil
	}
	return mailstubapi.GetMessage200JSONResponse{Body: mailstubapi.Message{
		Id:         stored.ID,
		To:         openapi_types.Email(stored.To),
		Subject:    stored.Subject,
		AcceptedAt: timestamp.Format(stored.AcceptedAt),
		Text:       stored.Text,
	}}, nil
}

// inboxScopeOf names the collection and the parameters a cursor of this operation is bound to. The
// limit is the one the page is read with rather than the one the request spelled, so a client that
// omits the parameter continues with the page size the first page used.
//
// The scope belongs to no account: the inbox is read anonymously, so its cursor binds the operation
// and its parameters rather than a subject no request ever stated.
func (h mailstubInboxHandlers) inboxScopeOf(limit int) cursor.Scope {
	return cursor.AnonymousOperationOn(getMessagesOperation,
		cursor.Parameter{Name: limitParameter, Value: strconv.Itoa(limit)})
}

// positionOf reads the position a presented cursor names, or nil when the request carries none.
// Every failure — an unreadable token, a signature that does not match its payload, a cursor issued
// for another operation or another parameter set, and one signed with the key of another
// installation — is the one refusal the contract declares for a cursor.
func (h mailstubInboxHandlers) positionOf(
	presented *mailstubapi.Cursor, limit int,
) (*cursor.Position, error) {
	if presented == nil {
		return nil, nil
	}
	position, err := h.cursors.Read(string(*presented), h.inboxScopeOf(limit))
	if err != nil {
		return nil, err
	}
	return &position, nil
}

// collectionBody renders one page of the box, with the cursor that reads the page after it. A last
// or empty page carries no cursor: there is nothing to continue from, and the contract publishes
// that as a null cursor.
func (h mailstubInboxHandlers) collectionBody(read readPage) (mailstubapi.MessageCollection, error) {
	items := make([]mailstubapi.MessageSummary, 0, len(read.messages))
	for _, stored := range read.messages {
		items = append(items, mailstubapi.MessageSummary{
			Id:         stored.ID,
			To:         openapi_types.Email(stored.To),
			Subject:    stored.Subject,
			AcceptedAt: timestamp.Format(stored.AcceptedAt),
		})
	}
	body := mailstubapi.MessageCollection{Items: items, NextCursor: read.next}
	return body, nil
}
