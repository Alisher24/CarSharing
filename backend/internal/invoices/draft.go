package invoices

import (
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/google/uuid"
)

// Draft is an invoice about to be written: the ride it describes, the lines of the charge it states and
// the total of them. It carries no identifier and no version, because those belong to the row that is
// created rather than to the decision that asks for it.
type Draft struct {
	RentalID   string
	UserID     uuid.UUID
	IssuedAt   time.Time
	Completion completion.Reason

	// Exhausted is what a ride that ran out of energy states about it: the sources that were empty
	// when it did. A ride that was ended by a person ran out of nothing and carries none.
	Exhausted []fleet.SourceKind

	Driving Line
	Paused  Line

	TotalTyiyn billing.AmountTyiyn
}
