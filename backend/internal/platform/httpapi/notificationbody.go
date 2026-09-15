package httpapi

import (
	"errors"
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/completion"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
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
		return body, body.FromReservationExpiringNotification(servedapi.ReservationExpiringNotification{
			Id:        stored.ID,
			Type:      servedapi.ReservationExpiring,
			CreatedAt: timestamp.Format(stored.CreatedAt),
			ExpiresAt: timestamp.Format(stored.ExpiresAt),
			RentalId:  stored.RentalID,
			Active:    stored.Active,
			ReadAt:    readAtOf(stored),
			Version:   exactInteger(stored.Version),
		})
	case notifications.RentalCompleted:
		completed, err := completionBody(stored.CompletionReason, stored.Exhausted)
		if err != nil {
			return servedapi.Notification{}, err
		}
		if stored.EndedAt == nil {
			return servedapi.Notification{}, errors.New(
				"a report of a finished ride states no moment the ride ended at")
		}
		return body, body.FromRentalCompletedNotification(servedapi.RentalCompletedNotification{
			Id:         stored.ID,
			Type:       servedapi.RentalCompleted,
			CreatedAt:  timestamp.Format(stored.CreatedAt),
			EndedAt:    timestamp.Format(*stored.EndedAt),
			RentalId:   stored.RentalID,
			InvoiceId:  stored.InvoiceID,
			Completion: completed,
			Active:     stored.Active,
			ReadAt:     readAtOf(stored),
			Version:    exactInteger(stored.Version),
		})
	default:
		return servedapi.Notification{}, fmt.Errorf(
			"a notification of kind %q cannot be published yet", stored.Kind)
	}
}

// readAtOf publishes the moment a notification was read, which a notification nobody has read does
// not state.
func readAtOf(stored notifications.Notification) *servedapi.Timestamp {
	if stored.ReadAt == nil {
		return nil
	}
	formatted := servedapi.Timestamp(timestamp.Format(*stored.ReadAt))
	return &formatted
}

// completionBody spells why a ride ended in the shape the contract declares for that reason. A reason
// this build does not publish is reported as a defect of the server rather than written under another
// reason's shape, so a stored row nobody can explain reaches no client.
//
// A ride that ran out of energy states which sources were empty when it did, and the contract requires
// at least one of them: an ending with no source to name is an ending this build did not write, and it
// is reported rather than published as an empty list.
func completionBody(reason completion.Reason, exhausted []fleet.SourceKind) (servedapi.Completion, error) {
	var body servedapi.Completion
	switch reason {
	case completion.UserFinished:
		return body, body.FromUserFinished(servedapi.UserFinished{
			Reason: servedapi.UserFinishedReasonUserFinished,
		})
	case completion.EnergyDepleted:
		if len(exhausted) == 0 {
			return servedapi.Completion{}, errors.New(
				"a ride that ran out of energy states no source it ran out of")
		}
		sources := make([]servedapi.SourceKind, 0, len(exhausted))
		for _, kind := range exhausted {
			sources = append(sources, servedapi.SourceKind(kind))
		}
		return body, body.FromEnergyDepleted(servedapi.EnergyDepleted{
			Reason:           servedapi.EnergyDepletedReasonEnergyDepleted,
			ExhaustedSources: sources,
		})
	default:
		return servedapi.Completion{}, fmt.Errorf("a completion reason of %q cannot be published", reason)
	}
}
