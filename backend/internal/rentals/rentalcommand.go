package rentals

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// RentalAction names what one command asks of one of the caller's rentals. Each spelling is the
// identifier the operation carrying it is named by, so a command, the operation that answers it and the
// log line a defect prints all name one thing.
type RentalAction string

const (
	// CancelRental gives a reservation back before its deadline.
	CancelRental RentalAction = "cancel"

	// StartRental turns a reservation into a ride that is driving.
	StartRental RentalAction = "start"

	// PauseRental makes a driving ride stand still.
	PauseRental RentalAction = "pause"

	// ResumeRental makes a paused ride drive again.
	ResumeRental RentalAction = "resume"
)

// RentalCommand names one of the caller's rentals and what is asked of it, whoever asks. Whether the
// caller may move it is decided from the rental the module reads rather than from anything the request
// carries.
type RentalCommand struct {
	Action   RentalAction
	Caller   uuid.UUID
	RentalID string
	Attempt  Attempt
}

// Apply decides one command about one of the caller's rentals, or answers why it cannot. Which commands
// exist and what each of them does is the table below rather than a branch here, so a new command is a
// new row.
func (s *Service) Apply(ctx context.Context, command RentalCommand) (Answered, error) {
	decide, known := rentalCommands[command.Action]
	if !known {
		return Answered{}, fmt.Errorf("the rental command %q is not one this module knows", command.Action)
	}
	return decide(s, ctx, command)
}

// rentalCommands is every command this module applies to one of the caller's rentals. The three that
// move a ride share one path because they differ in the transition they ask for rather than in anything
// they do.
var rentalCommands = map[RentalAction]func(*Service, context.Context, RentalCommand) (Answered, error){
	StartRental:  (*Service).rideAlong,
	PauseRental:  (*Service).rideAlong,
	ResumeRental: (*Service).rideAlong,
	CancelRental: (*Service).cancelReservation,
}
