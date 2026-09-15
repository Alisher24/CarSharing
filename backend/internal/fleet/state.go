package fleet

// Status is the vehicle state the public catalog publishes.
type Status string

const (
	Available   Status = "available"
	Reserved    Status = "reserved"
	InTrip      Status = "in_trip"
	Unavailable Status = "unavailable"
)

// RideMode is what a vehicle in a trip is doing.
type RideMode string

const (
	Driving RideMode = "driving"
	Paused  RideMode = "paused"
)

// UnavailableReason is why a free vehicle still cannot be rented. Every reason that applies is
// published, because one of them being fixed does not make the vehicle available.
type UnavailableReason string

const (
	InsufficientEnergy UnavailableReason = "insufficient_energy"
	TelemetryStale     UnavailableReason = "telemetry_stale"
	OutsideServiceZone UnavailableReason = "outside_service_zone"

	// ServiceRequired is a vehicle a ride ran out of energy in. It stays out of service until
	// somebody has looked at it, and refilling it does not put it back: the ride ended because its
	// reserves ran out, and servicing is the answer to that rather than a full tank.
	ServiceRequired UnavailableReason = "service_required"

	TechnicalUnavailable UnavailableReason = "technical_unavailable"
)

// State is everything the catalog says about what a vehicle is doing: the status, the ride mode
// when it is in a trip, and every reason it is unavailable when it is not held by a rental.
type State struct {
	Status             Status
	RideMode           RideMode
	UnavailableReasons []UnavailableReason
}
