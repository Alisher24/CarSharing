package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/google/uuid"
)

// FinishCommand ends one ride of one account, whoever asks. Whether the caller may end it is decided
// from the rental the module reads rather than from anything the request carries: the contract states
// that a finish uses confirmed server telemetry, so no coordinate, amount or owner travels with it.
type FinishCommand struct {
	Caller   uuid.UUID
	RentalID string
	Attempt  Attempt
}

// Finish ends the caller's ride and issues the invoice for it, or answers why it cannot.
//
// The whole ending is one transaction under the shared lock order: the account, the vehicle and the
// rental are locked, the relationships are read again under those locks, and the moment every boundary
// is judged by is read from the database afterwards. A ride another attempt has already ended is
// answered with that attempt's ending rather than written over, which is what makes two finishes of
// one ride produce one completed rental and one invoice.
func (s *Service) Finish(ctx context.Context, command FinishCommand) (Answered, error) {
	return s.answer(ctx, idempotency.ForAccount(command.Caller), command.Attempt,
		rideParticipants(s.pool, RideCommand{Caller: command.Caller, RentalID: command.RentalID}),
		func(ctx context.Context, moment time.Time) (Outcome, error) {
			return s.finishWithin(ctx, moment, command)
		})
}

// finishWithin decides one finish with the participants locked and the moment fixed.
func (s *Service) finishWithin(
	ctx context.Context, moment time.Time, command FinishCommand,
) (Outcome, error) {
	target, err := rentalByIDFor(ctx, s.pool, command.Caller, command.RentalID)
	if errors.Is(err, ErrRentalNotFound) {
		return refused(moment, Refusal{Kind: RentalNotFound}), nil
	}
	if err != nil {
		return Outcome{}, err
	}

	// The model is brought to the moment of the command before anything is judged. A ride whose
	// sources have run out is over whatever this command was going to say about it, and a command
	// that arrives after it did not end the ride: it found one already ended, and the reason and the
	// moment are the ones the model ran it out at. A ride that runs out at the very moment of the
	// command is answered the same way, which is what gives depletion priority over a finish of the
	// same instant.
	if _, err = s.reconcileVehicle(ctx, moment, target.VehicleID, &target); err != nil {
		return Outcome{}, err
	}
	target, err = rentalByIDFor(ctx, s.pool, command.Caller, command.RentalID)
	if err != nil {
		return Outcome{}, err
	}

	// A ride that has already ended is not ended again: the reason and the moment the ending
	// transaction stored are what a new key is answered with, together with the invoice that
	// transaction issued. Neither of them is replaced.
	if target.Stage == stage.Completed {
		return s.storedEnding(ctx, moment, target)
	}
	if refusal := finishRefusal(target); refusal != nil {
		return refused(moment, *refusal), nil
	}

	refusal, err := s.finishPrepared(ctx, moment, target)
	if err != nil {
		return Outcome{}, err
	}
	if refusal != nil {
		return refused(moment, *refusal), nil
	}
	return s.endRide(ctx, moment, target, Ending{Reason: finishReason, EndedAt: moment})
}

// finishPrepared reports what a finish requires of the vehicle before it may end the ride. Nothing is
// written when it refuses: the ride keeps its mode, its open interval and its charging, exactly as the
// product states for an ending that is not admissible where the vehicle stands.
func (s *Service) finishPrepared(
	ctx context.Context, moment time.Time, target Rental,
) (*Refusal, error) {
	vehicle, err := s.vehicles.VehicleAt(ctx, target.VehicleID, moment)
	if err != nil {
		return nil, err
	}
	return finishLandingRefusal(vehicle, target.ZoneID, moment, s.finishLanding), nil
}

// storedEnding answers a ride that has already ended with what the ending transaction stored: the
// rental as it stands and the invoice of that ride.
func (s *Service) storedEnding(ctx context.Context, moment time.Time, target Rental) (Outcome, error) {
	issued, err := s.invoices.ByRental(ctx, target.ID)
	if err != nil {
		return Outcome{}, err
	}
	vehicle, err := s.vehicles.VehicleAt(ctx, target.VehicleID, moment)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Rental: target, Vehicle: vehicle, Moment: moment, Invoice: issued}, nil
}

// finishReason is why this build ends a ride when a person ends it. The other reason the contract
// publishes belongs to the model that runs a ride out.
const finishReason = completion.UserFinished
