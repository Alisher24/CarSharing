package httpapi

import (
	"context"
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/google/uuid"
)

// Notifications is what the notification operations need: one page of the caller's own
// notifications, and the read that marks one of them. The module that answers the collection also
// fixes a warning that has become due for the reservation the caller still holds, so the transport
// asks for the page rather than assembling one from the store.
type Notifications interface {
	Collection(
		ctx context.Context, caller uuid.UUID, after *notifications.Position, limit int,
	) (notifications.Collection, error)

	MarkRead(ctx context.Context, owner uuid.UUID, id string) (notifications.Result, error)
}

// notificationHandlers answers the operations that belong to one signed-in person's own
// notifications.
type notificationHandlers struct {
	notifications Notifications
	cursors       *cursor.Signer
}

func newNotificationHandlers(
	notifications Notifications, cursors *cursor.Signer,
) (notificationHandlers, error) {
	if notifications == nil {
		return notificationHandlers{}, fmt.Errorf("%w: notification operations", ErrIncompleteApplication)
	}
	if cursors == nil {
		return notificationHandlers{}, fmt.Errorf("%w: cursor signer", ErrIncompleteApplication)
	}
	return notificationHandlers{notifications: notifications, cursors: cursors}, nil
}
