package rentals

import (
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

// finishRefusal reports why a finish does not apply to a rental, or nil when it does. A ride that has
// begun is ended wherever it is; a ride that has already ended is not a refusal but the stored ending
// itself, which finishWithin answers before this rule is asked.
func finishRefusal(target Rental) *Refusal {
	if target.Stage == stage.Active || target.Stage == stage.Paused {
		return nil
	}
	return &Refusal{Kind: InvalidRentalState}
}

// finishLandingRefusal reports why a ride may not be ended where its vehicle stands, or nil when it
// may. The position is the one the vehicle last confirmed rather than anything a client sent, so a
// finish is judged by the same coordinates the catalog publishes, and the area the ride was made in is
// the whole of where it may end.
func finishLandingRefusal(vehicle fleet.Vehicle, zoneID string, moment time.Time) *Refusal {
	if refusal := positionRefusal(vehicle, moment); refusal != nil {
		return refusal
	}
	if vehicle.ServiceZoneID == zoneID {
		return nil
	}
	return &Refusal{Kind: OutsideServiceZone}
}

// positionRefusal reports why the confirmed position of a vehicle may not decide a finish, or nil when
// it may. A vehicle the platform has no link to and one whose last reading is older than the limit are
// answered the same way: neither of them states where the vehicle is now, so the ride keeps going.
func positionRefusal(vehicle fleet.Vehicle, moment time.Time) *Refusal {
	if vehicle.TelemetryFreshnessAt(moment) == fleet.FreshTelemetry {
		return nil
	}
	return &Refusal{Kind: TelemetryStale}
}
