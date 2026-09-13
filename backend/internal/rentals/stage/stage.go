// Package stage is the vocabulary of what a rental is doing. It sits below the rentals module and
// the fleet catalog so that both name the same stages: the catalog derives what a visitor sees from
// the stage a rental reached, and the module that writes stages would otherwise have to be imported
// by the catalog it publishes through.
package stage

// Stage is how far a rental has got.
type Stage string

const (
	// Reserved, Active and Paused are the stages that hold a vehicle: exactly one rental of a
	// vehicle may be in one of them at a time.
	Reserved Stage = "reserved"
	Active   Stage = "active"
	Paused   Stage = "paused"

	// Cancelled, Expired and Completed have released the vehicle.
	Cancelled Stage = "cancelled"
	Expired   Stage = "expired"
	Completed Stage = "completed"
)

// NotHeld is the absence of a rental, which is what a vehicle nobody has taken is held by.
const NotHeld Stage = ""

// Holds reports whether a rental in this stage still holds its vehicle.
func (s Stage) Holds() bool {
	return s == Reserved || s == Active || s == Paused
}
