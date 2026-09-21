package rentals

import (
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/google/uuid"
)

// ErrRentalNotFound reports an identifier no rental of this account carries.
var ErrRentalNotFound = errors.New("no such rental")

// Rental is one rental as this module reads and writes it: the reservation it started as, the ride
// it may become, and the moment it released its vehicle.
type Rental struct {
	ID        string
	UserID    uuid.UUID
	VehicleID string
	Stage     stage.Stage
	Version   int64

	ReservedAt time.Time
	ExpiresAt  time.Time
	StartedAt  *time.Time
	EndedAt    *time.Time

	// CompletionReason is why the ride ended, which exactly the rentals that have ended carry.
	CompletionReason *completion.Reason

	// Exhausted is what a ride that ran out of energy states about it: the sources that were empty
	// when it did, in the order the vehicle's profile uses them. A ride a person ended carries none,
	// which is what the column states by holding nothing.
	Exhausted []fleet.SourceKind

	// ZoneID is the service area the vehicle stood in when the reservation was made. It is what a
	// finish is judged against: the ride may not be ended where the area does not cover it.
	ZoneID string

	// ModeStartedAt is the moment the ride entered the mode it is in, which is the moment the interval
	// it is in began. It is set exactly while the rental is a ride that has begun.
	ModeStartedAt *time.Time

	// Tariff is the price list this rental was made under, stored with the rental rather than read
	// from the catalog on demand: a later change of the catalog must not reach a reservation that
	// already exists.
	Tariff tariffs.Tariff
}

// Riding reports whether the rental is a ride that has begun, which is what an answer publishes with a
// mode moment and a progress.
func (r Rental) Riding() bool {
	return (r.Stage == stage.Active || r.Stage == stage.Paused) && r.ModeStartedAt != nil
}
