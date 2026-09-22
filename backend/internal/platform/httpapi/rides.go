package httpapi

import (
	"context"
	"fmt"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// The paths of the ride commands. They are the specification's own routes, stated once here so that
// every handler and every fingerprint names the same one. The identifier of the rental a command acts
// on stands where the contract puts it.
const (
	startRentalPath  = "/api/v1/reservations/{id}/start"
	pauseRentalPath  = "/api/v1/rides/{id}/pause"
	resumeRentalPath = "/api/v1/rides/{id}/resume"
)

// rideHandlers answers the commands that move a ride along its lifecycle. The three commands differ in
// the transition they ask the module for and in the operation they answer as, and in nothing else: each
// handler resolves its caller, builds the attempt of its own operation and hands the answer to the one
// way a ride answer is spelled.
type rideHandlers struct{ reservations Reservations }

func newRideHandlers(reservations Reservations) (rideHandlers, error) {
	if reservations == nil {
		return rideHandlers{}, fmt.Errorf("%w: ride commands", ErrIncompleteApplication)
	}
	return rideHandlers{reservations: reservations}, nil
}

// StartRental begins the ride of the caller's reservation.
func (h rideHandlers) StartRental(
	ctx context.Context, request servedapi.StartRentalRequestObject,
) (servedapi.StartRentalResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.StartRental401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	attempt, err := startRideOperation.attempt(ctx, string(request.Id),
		commandKeyHeader(request.Params.IdempotencyKey))
	if err != nil {
		return nil, err
	}
	answered, err := h.reservations.Apply(ctx, rentals.RentalCommand{
		Action:   rentals.StartRental,
		Caller:   caller,
		RentalID: string(request.Id),
		Attempt:  attempt,
	})
	spelled, err := rideAnswer(ctx, startRideOperation, answered, err)
	if err != nil {
		return nil, err
	}
	return spelled.(servedapi.StartRentalResponseObject), nil
}

// PauseRental makes the caller's driving ride stand still.
func (h rideHandlers) PauseRental(
	ctx context.Context, request servedapi.PauseRentalRequestObject,
) (servedapi.PauseRentalResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.PauseRental401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	attempt, err := pauseRideOperation.attempt(ctx, string(request.Id),
		commandKeyHeader(request.Params.IdempotencyKey))
	if err != nil {
		return nil, err
	}
	answered, err := h.reservations.Apply(ctx, rentals.RentalCommand{
		Action:   rentals.PauseRental,
		Caller:   caller,
		RentalID: string(request.Id),
		Attempt:  attempt,
	})
	spelled, err := rideAnswer(ctx, pauseRideOperation, answered, err)
	if err != nil {
		return nil, err
	}
	return spelled.(servedapi.PauseRentalResponseObject), nil
}

// ResumeRental makes the caller's paused ride drive again.
func (h rideHandlers) ResumeRental(
	ctx context.Context, request servedapi.ResumeRentalRequestObject,
) (servedapi.ResumeRentalResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.ResumeRental401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	attempt, err := resumeRideOperation.attempt(ctx, string(request.Id),
		commandKeyHeader(request.Params.IdempotencyKey))
	if err != nil {
		return nil, err
	}
	answered, err := h.reservations.Apply(ctx, rentals.RentalCommand{
		Action:   rentals.ResumeRental,
		Caller:   caller,
		RentalID: string(request.Id),
		Attempt:  attempt,
	})
	spelled, err := rideAnswer(ctx, resumeRideOperation, answered, err)
	if err != nil {
		return nil, err
	}
	return spelled.(servedapi.ResumeRentalResponseObject), nil
}

// rideAnswer spells a command that was decided, or the failure that kept it from being decided: the
// rental as it now stands, the refusal the module stored for this key, or an answer of this server.
func rideAnswer(
	ctx context.Context, operation commandOperation, answered rentals.Answered, failure error,
) (any, error) {
	if failure == nil {
		return answerOf(operation, answered)
	}
	reported := commandFailureOf(failure)
	reportUncarried(ctx, failure, reported)
	return spellFailure(operation, ctx, reported)
}

func registerRideHandlers(served *server, dependencies Dependencies) error {
	var err error
	served.rideHandlers, err = newRideHandlers(dependencies.Reservations)
	return err
}
