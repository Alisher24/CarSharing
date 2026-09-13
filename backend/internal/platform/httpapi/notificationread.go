package httpapi

import (
	"context"
	"errors"
	"log/slog"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
)

// ReadNotification marks one notification of the caller as read and answers it. The operation
// carries no command key: the notification stores the moment it was read once, so a repeated request
// answers the representation the first one stored rather than needing a key to recognize it.
func (h notificationHandlers) ReadNotification(
	ctx context.Context, request servedapi.ReadNotificationRequestObject,
) (servedapi.ReadNotificationResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.ReadNotification401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}

	result, err := h.notifications.MarkRead(ctx, caller, string(request.Id))
	if errors.Is(err, notifications.ErrNotFound) {
		// A notification of another account and one that never existed are answered alike, so the
		// answer cannot be used to learn that somebody else's notification exists.
		return servedapi.ReadNotification404JSONResponse{
			Body: apiErrorBody(ctx, codeResourceNotFound, messageResourceNotFound),
		}, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "the notification could not be marked read", "error", err)
		return servedapi.ReadNotification503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	body, err := notificationReadBody(result)
	if err != nil {
		return nil, err
	}
	return servedapi.ReadNotification200JSONResponse{Body: body}, nil
}

// notificationReadBody publishes the final notification together with the moment the read fixed.
func notificationReadBody(result notifications.Result) (servedapi.NotificationReadResult, error) {
	notification, err := notificationBody(result.Notification)
	if err != nil {
		return servedapi.NotificationReadResult{}, err
	}
	return servedapi.NotificationReadResult{
		Notification: notification,
		ServerTime:   timestamp.Format(result.Moment),
	}, nil
}
