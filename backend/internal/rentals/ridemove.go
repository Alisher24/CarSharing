package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StartRideCommand starts the ride of one reservation.
type StartRideCommand struct {
	Caller   uuid.UUID
	RentalID string
	Attempt  Attempt
}

// StartRide turns the caller's reservation into a ride that is driving, or answers why it cannot.
//
// The ride begins at the moment the transaction read from the database after its waits, which is the
// moment `started_at` and the first interval of the ride both carry: restarting the process and
// reading the rental again report the moment that was stored rather than a second one.
func (s *Service) StartRide(ctx context.Context, command StartRideCommand) (Answered, error) {
	return s.Ride(ctx, StartRide, RideCommand{
		Caller:   command.Caller,
		RentalID: command.RentalID,
		Attempt:  command.Attempt,
	})
}

// PauseRideCommand makes the ride of one rental stand still.
type PauseRideCommand struct {
	Caller   uuid.UUID
	RentalID string
	Attempt  Attempt
}

// PauseRide closes the interval the ride is driving in and opens one of standing still, both at the
// moment the transaction fixed after its waits, or answers why it cannot.
func (s *Service) PauseRide(ctx context.Context, command PauseRideCommand) (Answered, error) {
	return s.Ride(ctx, PauseRide, RideCommand{
		Caller:   command.Caller,
		RentalID: command.RentalID,
		Attempt:  command.Attempt,
	})
}

// ResumeRideCommand makes a paused ride drive again.
type ResumeRideCommand struct {
	Caller   uuid.UUID
	RentalID string
	Attempt  Attempt
}

// ResumeRide closes the interval the ride is standing still in and opens one of driving, both at the
// moment the transaction fixed after its waits, or answers why it cannot.
func (s *Service) ResumeRide(ctx context.Context, command ResumeRideCommand) (Answered, error) {
	return s.Ride(ctx, ResumeRide, RideCommand{
		Caller:   command.Caller,
		RentalID: command.RentalID,
		Attempt:  command.Attempt,
	})
}

// FinishRide ends the ride of one rental and issues the invoice for it.
func (s *Service) FinishRide(ctx context.Context, command FinishCommand) (Answered, error) {
	return s.Finish(ctx, command)
}

// PayInvoice settles the caller's invoice for a ride that has ended.
func (s *Service) PayInvoice(ctx context.Context, command PayCommand) (Answered, error) {
	return s.Pay(ctx, command)
}

// startRideStatement moves one reservation into a ride that is driving. The ride begins at the moment
// the transaction fixed, which becomes both the moment the ride started and the moment its first mode
// began: they are one instant, so they are one value.
const startRideStatement = `
UPDATE rentals
SET stage = $2, started_at = $3, mode_started_at = $3, version = version + 1
WHERE id = $1 AND stage = $4
RETURNING` + rentalFields

// changeModeStatement moves a ride between its two modes. It changes when the current mode began and
// nothing else: the ride started once and keeps the moment it did.
const changeModeStatement = `
UPDATE rentals
SET stage = $2, mode_started_at = $3, version = version + 1
WHERE id = $1 AND stage = $4
RETURNING` + rentalFields

// moveRide writes one transition of a ride and returns the rental as it now stands. The stage it moves
// from is part of the statement, so a rental another transaction has already moved is reported as
// unmoved rather than written over, and no interval is opened for it.
func moveRide(
	ctx context.Context, pool *pgxpool.Pool, target Rental, transition rideTransition, moment time.Time,
) (Rental, error) {
	statement := changeModeStatement
	if transition.from == stage.Reserved {
		statement = startRideStatement
	}
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, statement,
		target.ID, transition.to, moment, transition.from)
	if err != nil {
		return Rental{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Rental{}, err
		}
		return Rental{}, errRentalMoved
	}
	var moved Rental
	if err = scanRental(rows, &moved); err != nil {
		return Rental{}, err
	}
	return moved, rows.Err()
}

// errRentalMoved reports that the rental no longer stood in the stage the command was judged against.
var errRentalMoved = errors.New("the rental had already been moved")
