package rentals

import (
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// vehicleRefusal reports why a vehicle may not be reserved at an instant, or nil when it may. A
// vehicle somebody holds is refused before its own condition is judged: how full it is says nothing
// about whether it is free.
func vehicleRefusal(vehicle fleet.Vehicle, moment time.Time) *Refusal {
	if vehicle.HeldBy.Holds() {
		return &Refusal{Kind: VehicleUnavailable}
	}
	state := vehicle.StateAt(moment)
	if state.Status != fleet.Available {
		return &Refusal{
			Kind:               VehicleUnavailable,
			UnavailableReasons: state.UnavailableReasons,
		}
	}
	return nil
}
