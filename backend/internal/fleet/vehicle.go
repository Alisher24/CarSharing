package fleet

import (
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// Vehicle is one vehicle as the public catalog sees it. It carries no renter, route, invoice or
// history: the rental behind HeldBy is named only by the stage it is in.
type Vehicle struct {
	ID             string
	Model          string
	PowertrainType PowertrainType
	Version        int64

	// Connected is the vehicle's link to the platform. A vehicle without one reports offline
	// however recent its last reading was.
	Connected bool

	Telemetry Telemetry

	// InsideServiceZone is whether the last confirmed position is covered by a service zone,
	// boundary included.
	InsideServiceZone bool

	Sources []EnergySource

	// HeldBy is the stage of the rental that currently holds the vehicle, or rentals.NotHeld when
	// no rental does.
	HeldBy rentals.Stage
}

// heldStates is what the catalog publishes for a vehicle a rental holds. A vehicle in one of these
// stages is occupied whatever its energy, telemetry or technical condition says.
var heldStates = map[rentals.Stage]State{
	rentals.Reserved: {Status: Reserved},
	rentals.Active:   {Status: InTrip, RideMode: Driving},
	rentals.Paused:   {Status: InTrip, RideMode: Paused},
}

// StateAt derives what a reader is shown at an instant. It is computed on every read from the
// rental that holds the vehicle and from the vehicle's own fitness, so there is no stored status
// that could disagree with either.
func (v Vehicle) StateAt(observedAt time.Time) State {
	if held, occupied := heldStates[v.HeldBy]; occupied {
		return held
	}
	reasons := v.unavailableReasonsAt(observedAt)
	if len(reasons) == 0 {
		return State{Status: Available}
	}
	return State{Status: Unavailable, UnavailableReasons: reasons}
}

// TelemetryFreshnessAt reports how far the published position can be trusted at an instant.
func (v Vehicle) TelemetryFreshnessAt(observedAt time.Time) Freshness {
	if !v.Connected {
		return OfflineTelemetry
	}
	if observedAt.Sub(v.Telemetry.ConfirmedAt) > MaxTelemetryAge {
		return StaleTelemetry
	}
	return FreshTelemetry
}

// CanStart reports whether a rental of this vehicle could begin on this source alone. A source the
// powertrain cannot move the vehicle with never qualifies, however full it is.
func (v Vehicle) CanStart(source EnergySource) bool {
	profile, known := ProfileOf(v.PowertrainType)
	return known && profile.DrivesAlone(source.Kind) && source.MeetsStartThreshold()
}

// FitToStart reports whether any one source on its own satisfies the start threshold.
func (v Vehicle) FitToStart() bool {
	for _, source := range v.Sources {
		if v.CanStart(source) {
			return true
		}
	}
	return false
}

// unavailabilityCheck is one condition that keeps a free vehicle from being rented.
type unavailabilityCheck struct {
	reason  UnavailableReason
	applies func(Vehicle, time.Time) bool
}

// unavailabilityChecks are every such condition, in the order the catalog lists them, so that one
// vehicle's reasons are always spelled in the same sequence. A new condition is a new entry here.
var unavailabilityChecks = []unavailabilityCheck{
	{
		reason:  InsufficientEnergy,
		applies: func(vehicle Vehicle, _ time.Time) bool { return !vehicle.FitToStart() },
	},
	{
		reason: TelemetryStale,
		applies: func(vehicle Vehicle, observedAt time.Time) bool {
			return vehicle.TelemetryFreshnessAt(observedAt) == StaleTelemetry
		},
	},
	{
		reason: TechnicalUnavailable,
		applies: func(vehicle Vehicle, observedAt time.Time) bool {
			return vehicle.TelemetryFreshnessAt(observedAt) == OfflineTelemetry
		},
	},
	{
		reason:  OutsideServiceZone,
		applies: func(vehicle Vehicle, _ time.Time) bool { return !vehicle.InsideServiceZone },
	},
}

// unavailableReasonsAt lists every reason that applies at once, because a card that named only the
// first would suggest the vehicle becomes available as soon as that one is fixed.
func (v Vehicle) unavailableReasonsAt(observedAt time.Time) []UnavailableReason {
	var reasons []UnavailableReason
	for _, check := range unavailabilityChecks {
		if check.applies(v, observedAt) {
			reasons = append(reasons, check.reason)
		}
	}
	return reasons
}
