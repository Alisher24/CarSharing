package httpapi

import (
	"context"
	"log/slog"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/google/uuid"
)

// getNotificationsOperation is the name the signed cursors of this collection are bound to: the
// operation's own identifier in the contract.
const getNotificationsOperation = "getNotifications"

// GetNotifications answers one page of the caller's own notifications, newest first. Everything a
// page depends on comes from the session or from the request: the owner is never taken from a
// parameter, so another account's notifications cannot be asked for, and the parameters a cursor is
// bound to are the ones this request states.
func (h notificationHandlers) GetNotifications(
	ctx context.Context, request servedapi.GetNotificationsRequestObject,
) (servedapi.GetNotificationsResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.GetNotifications401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	limit := pageLimitOf(request.Params.Limit, notifications.PageSize)

	after, err := h.positionOf(request.Params.Cursor, caller, limit)
	if err != nil {
		return servedapi.GetNotifications400JSONResponse{
			Body: apiErrorBody(ctx, codeInvalidCursor, messageInvalidCursor),
		}, nil
	}
	page, err := h.notifications.Collection(ctx, caller, after, limit)
	if err != nil {
		slog.ErrorContext(ctx, "the notification collection could not be read", "error", err)
		return servedapi.GetNotifications503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	body, err := h.collectionBody(page, caller, limit)
	if err != nil {
		return nil, err
	}
	return servedapi.GetNotifications200JSONResponse{Body: body}, nil
}

// positionOf reads the position a presented cursor names, in the vocabulary of the module that pages
// the collection.
func (h notificationHandlers) positionOf(
	presented *servedapi.Cursor, owner uuid.UUID, limit int,
) (*notifications.Position, error) {
	read, err := pagePositionOf(h.cursors, presented,
		accountPageScope(getNotificationsOperation, owner, limit))
	if err != nil || read == nil {
		return nil, err
	}
	return &notifications.Position{CreatedAt: read.CreatedAt, ID: read.ID}, nil
}

// collectionBody renders one page of the collection, with the cursor that reads the page after it. A
// last or empty page carries no cursor: there is nothing to continue from, and the contract
// publishes that as a null cursor.
func (h notificationHandlers) collectionBody(
	page rentals.NotificationPage, owner uuid.UUID, limit int,
) (servedapi.NotificationCollection, error) {
	items := make([]servedapi.Notification, 0, len(page.Notifications))
	for _, stored := range page.Notifications {
		item, err := notificationBody(stored)
		if err != nil {
			return servedapi.NotificationCollection{}, err
		}
		items = append(items, item)
	}
	body := servedapi.NotificationCollection{
		Items:      items,
		ServerTime: timestamp.Format(page.Moment),
	}
	if page.Next == nil {
		return body, nil
	}
	issued, err := h.cursors.Issue(cursor.Position{
		CreatedAt: page.Next.CreatedAt,
		ID:        page.Next.ID,
	}, accountPageScope(getNotificationsOperation, owner, limit))
	if err != nil {
		return servedapi.NotificationCollection{}, err
	}
	body.NextCursor = &issued
	return body, nil
}
