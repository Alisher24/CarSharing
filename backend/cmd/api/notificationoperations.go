package main

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/google/uuid"
)

// notificationOperations is the two halves of the notification surface as one dependency: the
// collection belongs to the rentals module, because reading it fixes a warning that has become due
// for the reservation the reader holds, and the read of one notification belongs to the
// notifications module. The composition root is where the two are joined; neither module learns
// about the other's records.
type notificationOperations struct {
	reservations *rentals.Service
	collection   *notifications.Store
	reads        *notifications.Service
}

func (o notificationOperations) Collection(
	ctx context.Context, caller uuid.UUID, after *cursor.Position, limit int,
) (notifications.Collection, error) {
	var collection notifications.Collection
	read := func(txCtx context.Context, moment time.Time) error {
		return o.readCollection(txCtx, caller, after, limit, moment, &collection)
	}
	err := o.reservations.WithNotificationRead(ctx, caller, read)
	return collection, err
}

func (o notificationOperations) readCollection(
	ctx context.Context,
	caller uuid.UUID,
	after *cursor.Position,
	limit int,
	moment time.Time,
	collection *notifications.Collection,
) error {
	page, err := o.collection.ReadPage(ctx, caller, after, limit)
	if err != nil {
		return err
	}
	*collection = notifications.Collection{
		Notifications: page.Notifications,
		Next:          page.Next,
		Moment:        moment,
	}
	return nil
}

func (o notificationOperations) MarkRead(
	ctx context.Context, owner uuid.UUID, id string,
) (notifications.Result, error) {
	return o.reads.MarkRead(ctx, owner, id)
}
