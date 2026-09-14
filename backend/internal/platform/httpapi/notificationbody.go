package httpapi

import (
	"fmt"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
)

// notificationBody renders one stored notification in the shape its kind declares. The contract
// declares a notification as the discriminator of its kinds, so a kind this build does not serve is
// reported as a defect of the server rather than published under another kind's shape.
func notificationBody(stored notifications.Notification) (servedapi.Notification, error) {
	var body servedapi.Notification
	switch stored.Kind {
	case notifications.ReservationExpiring:
		var readAt *servedapi.Timestamp
		if stored.ReadAt != nil {
			formatted := servedapi.Timestamp(timestamp.Format(*stored.ReadAt))
			readAt = &formatted
		}
		return body, body.FromReservationExpiringNotification(servedapi.ReservationExpiringNotification{
			Id:        stored.ID,
			Type:      servedapi.ReservationExpiring,
			CreatedAt: timestamp.Format(stored.CreatedAt),
			ExpiresAt: timestamp.Format(stored.ExpiresAt),
			RentalId:  stored.RentalID,
			Active:    stored.Active,
			ReadAt:    readAt,
			Version:   exactInteger(stored.Version),
		})
	default:
		return servedapi.Notification{}, fmt.Errorf(
			"a notification of kind %q cannot be published yet", stored.Kind)
	}
}
