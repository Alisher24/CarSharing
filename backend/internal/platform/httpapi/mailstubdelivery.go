package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	mailstubapi "github.com/Alisher24/CarSharing/backend/internal/contracts/mailstubapi"
	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
)

// messageDeliveryConflict is what a delivery refused under a key that already holds another letter
// answers with. A client translates the codes it knows, so the message describes the outcome rather
// than addressing the sender.
const messageDeliveryConflict = "The delivery key already holds another letter"

// mailReplayed states whether one answer is a repeat. The mail stub's contract declares the header on
// both outcomes of a keyed operation rather than only on a repeat, so a client is told which of the
// two it received instead of reading an absent header as an answer of its own.
func mailReplayed(replayed bool) *bool { return &replayed }

// mailstubInternalHandlers answers the two operations of the internal listener: the delivery of one
// letter and the demonstration control that arms the loss of an answer.
type mailstubInternalHandlers struct {
	mailstubUnserved

	deliveries MailDeliveries
	actions    MailActions
}

// DeliverMessage stores one letter under the key it was delivered with and answers its receipt.
//
// The answer states what the box holds rather than what the request said: the identifier and the
// moment of the letter the key holds are read from the stored message, so a repeat is answered with
// the receipt of the first delivery and the box never shows one letter accepted twice.
func (h mailstubInternalHandlers) DeliverMessage(
	ctx context.Context, request mailstubapi.DeliverMessageRequestObject,
) (mailstubapi.DeliverMessageResponseObject, error) {
	if request.Body == nil {
		return nil, errors.New("a delivery must carry the letter it delivers")
	}
	key, err := mailstub.ParseKey(request.Params.DeliveryKey)
	if err != nil {
		// The header is one the contract declares a pattern for, so a value that does not fit it is
		// a malformed header rather than a letter this stub refuses to store.
		return mailstubapi.DeliverMessage400JSONResponse{
			Body: mailstubError(ctx, mailstubapi.INVALIDHEADER, messageInvalidHeader),
		}, nil
	}
	receipt, err := h.deliveries.Accept(ctx, key, mailstub.Request{
		To:      string(request.Body.To),
		Subject: request.Body.Subject,
		Text:    request.Body.Text,
	})
	if errors.Is(err, mailstub.ErrDeliveryConflict) {
		return mailstubapi.DeliverMessage409JSONResponse{
			Body: mailstubError(ctx, mailstubapi.DELIVERYCONFLICT, messageDeliveryConflict),
		}, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "the letter could not be stored", "error", err)
		return mailstubapi.DeliverMessage503JSONResponse{Body: mailstubUnavailable(ctx)}, nil
	}
	receipt201 := mailstubapi.DeliverMessage201JSONResponse{
		Body: mailstubapi.DeliveryReceipt{
			Id:         receipt.Message.ID,
			AcceptedAt: timestamp.Format(receipt.Message.AcceptedAt),
		},
		Headers: mailstubapi.DeliverMessage201ResponseHeaders{
			IdempotencyReplayed: mailReplayed(receipt.Replayed),
		},
	}
	if !receipt.Dropped {
		return receipt201, nil
	}
	// The letter is stored and the answer is the one a demonstration asked to lose, so the
	// connection is closed instead of the receipt being written. The header that would have marked
	// the repeat is part of the answer that was lost.
	return lostAnswer{}, nil
}

// lostAnswer is the answer of a delivery whose response was deliberately lost. Writing it means
// closing the connection: a client sees a broken transfer rather than a status, which is exactly the
// state a retry has to recover from.
//
// It is a response object of the operation rather than a step of the handler, because the layer that
// knows the answer was lost is the one that writes answers, and the handler has already decided
// everything the answer says.
type lostAnswer struct{}

// VisitDeliverMessageResponse closes the connection the answer would have been written on. A
// connection that cannot be taken over is reported as a failure rather than answered: writing the
// receipt would tell the client that a delivery it was asked to see lost had been acknowledged.
func (lostAnswer) VisitDeliverMessageResponse(w http.ResponseWriter) error {
	connection, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("the connection of a lost answer cannot be closed")
	}
	taken, _, err := connection.Hijack()
	if err != nil {
		return err
	}
	return taken.Close()
}

// ApplyMailDemoAction arms the loss of the next answer under the identifier it was asked with, and
// answers with the moment it did so. A repeat of that identifier reproduces the stored answer and
// arms nothing a second time.
func (h mailstubInternalHandlers) ApplyMailDemoAction(
	ctx context.Context, request mailstubapi.ApplyMailDemoActionRequestObject,
) (mailstubapi.ApplyMailDemoActionResponseObject, error) {
	if request.Body == nil {
		return nil, errors.New("a demonstration action must carry its identifier and what it asks for")
	}
	if !mailstub.Action(request.Body.Action).Known() {
		return nil, errors.New("a demonstration action this build does not apply reached the handler")
	}
	fingerprint, err := commandFingerprintOf(http.MethodPost, mailDemoActionPath, request.Body)
	if err != nil {
		return nil, err
	}
	result, replayed, err := h.actions.Apply(ctx, request.Body.ActionId, fingerprint)
	switch {
	case errors.Is(err, mailstub.ErrActionConflict):
		return mailstubapi.ApplyMailDemoAction409JSONResponse{
			Body: mailstubError(ctx, mailstubapi.IDEMPOTENCYCONFLICT, messageIdempotencyConflict),
		}, nil
	case errors.Is(err, mailstub.ErrActionInProgress):
		return mailstubapi.ApplyMailDemoAction409JSONResponse{
			Body: mailstubError(ctx, mailstubapi.IDEMPOTENCYINPROGRESS, messageIdempotencyInProgress),
			Headers: mailstubapi.ApplyMailDemoAction409ResponseHeaders{
				RetryAfter: retryAfterCommandBusy(),
			},
		}, nil
	case err != nil:
		slog.ErrorContext(ctx, "the demonstration action could not be applied", "error", err)
		return mailstubapi.ApplyMailDemoAction503JSONResponse{Body: mailstubUnavailable(ctx)}, nil
	}
	return mailstubapi.ApplyMailDemoAction200JSONResponse{
		Body: mailstubapi.MailDemoResult{
			ActionId:   result.ActionID,
			ServerTime: timestamp.Format(result.ServerTime),
		},
		Headers: mailstubapi.ApplyMailDemoAction200ResponseHeaders{
			IdempotencyReplayed: mailReplayed(replayed),
		},
	}, nil
}

// mailstubUnavailable is the body of every 503 of this surface: the process could not reach its own
// box, which is an outage rather than an answer about a letter.
func mailstubUnavailable(ctx context.Context) mailstubapi.ApiError {
	return mailstubError(ctx, mailstubapi.SERVICEUNAVAILABLE, messageServiceUnavailable)
}
