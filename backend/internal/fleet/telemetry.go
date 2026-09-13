package fleet

import "time"

// MaxTelemetryAge is how old a confirmed reading may be and still be trusted. A reading of exactly
// this age is fresh; the next microsecond is not.
const MaxTelemetryAge = 15 * time.Second

// Freshness is how much a published position can be relied on.
type Freshness string

const (
	// FreshTelemetry is a reading the vehicle confirmed within MaxTelemetryAge.
	FreshTelemetry Freshness = "fresh"

	// StaleTelemetry is a linked vehicle whose last confirmed reading is older than that.
	StaleTelemetry Freshness = "stale"

	// OfflineTelemetry is a vehicle the platform has no link to at all, whatever its last reading
	// said. It is not the same as a browser that cannot reach the API, which is the reader's own
	// connection and is reported separately.
	OfflineTelemetry Freshness = "offline"
)

// Position is a WGS84 point. Longitude comes first, as it does everywhere the contract writes a
// coordinate pair.
type Position struct {
	Longitude float64
	Latitude  float64
}

// Telemetry is the last reading a vehicle confirmed. Nothing predicts a position between two
// confirmations: the catalog publishes what the vehicle last reported.
type Telemetry struct {
	Position    Position
	ConfirmedAt time.Time
}
