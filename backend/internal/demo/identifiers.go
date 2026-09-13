package demo

import "fmt"

// identifierPrefix is the fixed head of every demonstration identifier. It is a UUIDv7 with the
// version and variant the contract requires of a stored resource, so demonstration rows are named
// the same way real ones are.
const identifierPrefix = "01994342-6ba7-7000-8000"

// The families demonstration identifiers are drawn from. Keeping them apart means a vehicle and
// the rental prepared for it never collide even though both are numbered from one.
const (
	vehicleFamily = iota + 1
	zoneFamily
	tariffFamily
	rentalFamily
)

// resourceID renders a deterministic identifier, so a seed run and a later restore always name the
// same row rather than adding a second copy of it.
func resourceID(family, index int) string {
	return fmt.Sprintf("%s-%04d%08d", identifierPrefix, family, index)
}
