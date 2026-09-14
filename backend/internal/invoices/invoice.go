// Package invoices records what a finished ride cost. An invoice is written once, by the same
// transaction that ends the ride, and read back by whoever is allowed to see it: nothing here prices a
// ride, opens a transaction of its own, sends anything or reads the environment. What it stores it
// states, and what it states it never changes.
package invoices

import (
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/google/uuid"
)

// Line is one mode of a finished ride as an invoice states it: how long the ride spent in that mode,
// the minutes begun in it, the rate those minutes were priced at and what they cost.
//
// The duration is counted in whole microseconds rather than held as a time.Duration. That is the unit
// the contract publishes and the unit the column stores, and ninety seconds expressed in nanoseconds is
// a different number: keeping a stored value in the unit it is stored in is what makes an invoice read
// back the digits it was written with.
//
// The amount is the product of the other two and is therefore not a column of its own: an invoice that
// stored it could state an amount its own rate and minutes do not produce, and no reader could say
// which of the three was the mistake.
type Line struct {
	Mode        billing.Mode
	Duration    billing.Microseconds
	Minutes     int64
	Rate        billing.RateTyiynPerStartedMinute
	AmountTyiyn billing.AmountTyiyn
}

// Invoice is what one finished ride cost under the price list its reservation stored.
//
// It carries its own copy of the rates and of the policy rather than reading them through the rental:
// an invoice states what it was issued at, and a later change of the rental — or of the catalog — must
// not change what a stored invoice says.
//
// Its two lines are stored as columns rather than as rows of a table of their own. The contract fixes
// exactly two of them, driving first and paused second, including a mode the ride never entered, so a
// row per line would be a second declaration of a list the contract already states and could hold a
// third.
type Invoice struct {
	ID       string
	RentalID string
	UserID   uuid.UUID

	IssuedAt      time.Time
	Currency      string
	BillingPolicy string
	Completion    completion.Reason
	Version       int64

	Driving Line
	Paused  Line

	TotalTyiyn billing.AmountTyiyn

	// Payment is the state of what is owed on this invoice: the status, the version of the view it is
	// published through, and the moment that view last moved.
	Payment          PaymentStatus
	PaymentVersion   int64
	PaymentUpdatedAt time.Time

	// PaidAt is the moment the invoice was settled and FailedAt the moment an attempt at it was
	// refused, with FailureCode the reason that refusal is published under. A payment that has not
	// reached a state carries no moment and no reason of it, and one absence is what says so: what is
	// not there is absent rather than held as the zero instant, which is a moment of the year one and
	// not a missing one.
	//
	// Each of the three is a pointer for the same reason: the columns admit null, and a value type
	// cannot hold the absence the row states.
	PaidAt      *time.Time
	FailedAt    *time.Time
	FailureCode *FailureCode
}

// Lines is the two lines of an invoice in the order the contract publishes them.
func (i Invoice) Lines() (driving Line, paused Line) { return i.Driving, i.Paused }

// CompletionReason is why the ride this invoice describes ended.
func (i Invoice) CompletionReason() completion.Reason { return i.Completion }

// LineOf reads one line of a charge as an invoice stores it, taking the duration in the unit the
// invoice counts in rather than in the nanoseconds a time.Duration holds.
func LineOf(line billing.Line) Line {
	return Line{
		Mode:        line.Mode,
		Duration:    line.Microseconds(),
		Minutes:     line.Minutes,
		Rate:        line.Rate,
		AmountTyiyn: line.AmountTyiyn,
	}
}
