package rentals

import (
	"context"
	"errors"

	"github.com/google/uuid"
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

// errRentalMoved reports that the rental no longer stood in the stage the command was judged against.
var errRentalMoved = errors.New("the rental had already been moved")
