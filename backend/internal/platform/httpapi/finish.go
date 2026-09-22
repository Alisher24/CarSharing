package httpapi

import (
	"context"
	"fmt"
	"net/http"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// finishRentalPath is the route of the command that ends a ride. It is the specification's own path,
// stated once here so that the handler and the fingerprint of the command name the same one.
const finishRentalPath = "/api/v1/rides/{id}/finish"

// finishHandlers answers the command that ends a ride. A finish differs from the three commands that
// move a ride in what it produces — a completed rental and the invoice of its ride — and in nothing
// else: the caller is resolved, the attempt of the operation is built, and the answer is spelled the
// one way a command answer is.
type finishHandlers struct{ reservations Reservations }

func newFinishHandlers(reservations Reservations) (finishHandlers, error) {
	if reservations == nil {
		return finishHandlers{}, fmt.Errorf("%w: finishing a ride", ErrIncompleteApplication)
	}
	return finishHandlers{reservations: reservations}, nil
}

// FinishRental ends the caller's ride and answers with the invoice it produced.
func (h finishHandlers) FinishRental(
	ctx context.Context, request servedapi.FinishRentalRequestObject,
) (servedapi.FinishRentalResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.FinishRental401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	attempt, err := finishRentalOperation.attemptOf(string(request.Id),
		commandKeyHeader(request.Params.IdempotencyKey), finishRender(ctx))
	if err != nil {
		return nil, err
	}
	answered, err := h.reservations.FinishRide(ctx, rentals.FinishCommand{
		Caller:   caller,
		RentalID: string(request.Id),
		Attempt:  attempt,
	})
	spelled, err := finishAnswer(ctx, answered, err)
	if err != nil {
		return nil, err
	}
	return spelled.(servedapi.FinishRentalResponseObject), nil
}

// finishAnswer spells a finish that was decided, or the failure that kept it from being decided: the
// completed rental with its invoice, the refusal the module stored for this key, or an answer of this
// server.
func finishAnswer(ctx context.Context, answered rentals.Answered, failure error) (any, error) {
	if failure == nil {
		return answerOf(finishRentalOperation, answered)
	}
	reported := commandFailureOf(failure)
	reportUncarried(ctx, failure, reported)
	return spellFailure(finishRentalOperation, ctx, reported)
}

// finishRender spells what a finish decided: the ride as it ended together with the invoice of it, and
// the refusal it decided on instead.
func finishRender(ctx context.Context) rentals.Render {
	return func(outcome rentals.Outcome) (rentals.Response, error) {
		if outcome.Refused() {
			return refusalRender(ctx, finishRentalOperation, outcome.Refusal)
		}
		body, err := finishedBody(outcome)
		if err != nil {
			return rentals.Response{}, err
		}
		return encoded(http.StatusOK, body)
	}
}

func registerFinishHandlers(served *server, dependencies Dependencies) error {
	var err error
	served.finishHandlers, err = newFinishHandlers(dependencies.Reservations)
	return err
}
