package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationPage is one page of an account's notifications as this module answers it: the records,
// the moment the read fixed, and where the page after it starts. An absent position is the last
// page, which is what the collection publishes as a null cursor.
type NotificationPage struct {
	Notifications []notifications.Notification
	Moment        time.Time
	Next          *notifications.Position
}

// Collection answers one page of the caller's own notifications, newest first, and fixes a warning
// that has become due for the reservation the caller still holds.
//
// The read is not a pure projection, for the same reason the read of what is current is not: a
// reservation inside its last minute has a warning due whether or not the worker is running, and a
// person who opens the application with the worker stopped must still be told. The transition is the
// one the worker performs, reached under the same lock order with the moment the transaction fixed,
// so a read and a sweep arriving together produce one warning rather than two.
func (s *Service) Collection(
	ctx context.Context, caller uuid.UUID, after *notifications.Position, limit int,
) (NotificationPage, error) {
	var page NotificationPage
	err := transact(ctx, s.pool, notificationParticipants(s.pool, caller),
		func(txCtx context.Context, moment time.Time) error {
			held, err := liveRentalAt(txCtx, s.pool, moment, userLiveRentalSelection, caller)
			if err != nil {
				return err
			}
			if held != nil {
				if _, err = createDueWarning(txCtx, s.pool, *held, moment); err != nil {
					return err
				}
			}
			read, err := notifications.NewStore(s.pool).ReadPage(txCtx, caller, after, limit)
			if err != nil {
				return err
			}
			page = NotificationPage{
				Notifications: read.Notifications,
				Moment:        moment,
				Next:          read.Next,
			}
			return nil
		})
	if err != nil {
		return NotificationPage{}, err
	}
	return page, nil
}

// notificationParticipants is the rows this read touches: the account itself, and the reservation it
// still holds. The vehicle is not among them, because nothing here reads the catalog: taking a row
// another transaction is waiting for would make this read wait for a transition it does not depend
// on.
func notificationParticipants(
	pool *pgxpool.Pool, caller uuid.UUID,
) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		planned := participants{users: []uuid.UUID{caller}}
		held, err := liveRentalOf(ctx, pool, userLiveRentalSelection, caller)
		if err != nil || held == nil {
			return planned, err
		}
		planned.rentals = sortedIdentifiers(held.ID)
		return planned, nil
	}
}
