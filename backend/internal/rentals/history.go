package rentals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrRideWithoutInvoice reports a finished ride no invoice was written for. The database admits one
// invoice per ride but does not require one, and the history publishes the invoice of every ride it
// lists, so such a row is reported rather than skipped: a history that silently loses somebody's ride
// is worse than a refusal that can be seen.
var ErrRideWithoutInvoice = errors.New("a finished ride carries no invoice")

// RideVehicle is the vehicle of a finished ride as the history names it: what it was, without the
// position or the energy the catalog publishes about a vehicle that exists now.
type RideVehicle struct {
	ID             string
	Model          string
	PowertrainType fleet.PowertrainType
}

// Ride is one finished ride of an account: the vehicle it was taken on, the moments it ran between,
// why it ended, and the invoice it was charged by.
//
// A ride is a rental that reached the completed stage with a ride that had begun. The moment it
// completed is the moment that rental released its vehicle, which is the only ending a completed
// stage has.
type Ride struct {
	ID          string
	Vehicle     RideVehicle
	StartedAt   time.Time
	CompletedAt time.Time
	InvoiceID   string
	Completion  completion.Reason

	// Exhausted is what a ride that ran out of energy states about it: the sources that were empty
	// when it did. A ride a person ended carries none.
	Exhausted []fleet.SourceKind
}

// RidePage is one page of an account's history: the rides it holds, and the position the page after
// it starts from. The position is absent on the last and on the empty page, which is the
// `next_cursor: null` the contract declares.
type RidePage struct {
	Rides []Ride
	Next  *cursor.Position
}

// Rides answers one page of the caller's own finished rides, newest first. The owner is the account
// the caller signed in as rather than anything the request carries, so no page can name another
// account's rides.
//
// The read fixes nothing and takes no lock: a ride that has ended is history, and the transitions
// that end one belong to the commands and the sweep that perform them.
func (s *Service) Rides(
	ctx context.Context, caller uuid.UUID, after *cursor.Position, limit int,
) (RidePage, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, ridePageSelection,
		caller, after.MomentArgument(), after.IdentifierArgument(), limit+1)
	if err != nil {
		return RidePage{}, err
	}
	defer rows.Close()

	found := make([]Ride, 0, limit+1)
	for rows.Next() {
		ride, err := scanRide(rows)
		if err != nil {
			return RidePage{}, err
		}
		found = append(found, ride)
	}
	if err = rows.Err(); err != nil {
		return RidePage{}, err
	}
	return ridePageOf(found, limit), nil
}

// ridePageOf cuts the rides read into the page that is published and the position the page after it
// starts from. One ride more than the page holds is read, so whether anything follows is decided by
// the database rather than by the page being shorter than the limit.
func ridePageOf(found []Ride, limit int) RidePage {
	published, next := cursor.Cut(found, limit, ridePosition)
	return RidePage{Rides: published, Next: next}
}

// scanRide reads one row of the history. Only the invoice is read as an absence the row admits: the
// selection states that a ride began and ended, and the table states that an ending carries a reason,
// so a null in any of those three is a failure of the read rather than a case to publish.
//
// A row whose ride has no invoice is reported with the ride it belongs to, so the defect can be found
// rather than only counted.
func scanRide(rows pgx.Rows) (Ride, error) {
	var (
		ride      Ride
		exhausted []string
		invoiceID *string
	)
	err := rows.Scan(
		&ride.ID,
		&ride.StartedAt,
		&ride.CompletedAt,
		&ride.Completion,
		&exhausted,
		&ride.Vehicle.ID,
		&ride.Vehicle.Model,
		&ride.Vehicle.PowertrainType,
		&invoiceID,
	)
	if err != nil {
		return Ride{}, err
	}
	if invoiceID == nil {
		return Ride{}, fmt.Errorf("%w: rental %s", ErrRideWithoutInvoice, ride.ID)
	}
	ride.InvoiceID = *invoiceID
	ride.Exhausted = fleet.SourceKinds(exhausted)
	return ride, nil
}

func ridePosition(item Ride) cursor.Position {
	return cursor.Position{Moment: item.CompletedAt, ID: item.ID}
}
