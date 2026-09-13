package rentals

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
