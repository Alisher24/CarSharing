package rentals

import (
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
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
// finish is judged by the same coordinates the catalog publishes.
func finishLandingRefusal(
	vehicle fleet.Vehicle, zoneID string, moment time.Time, landing string,
) *Refusal {
	if landing == config.TestRideLifecycleFinishLanding {
		// The rule the profile relaxed is where the ride stands, not whether its position can be
		// trusted at all: a ride whose vehicle is not reporting is still one no finish can be judged
		// by, and a check that relaxed both would prove nothing about either.
		return positionRefusal(vehicle, moment)
	}
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
