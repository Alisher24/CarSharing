package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/go-chi/chi/v5/middleware"
)

// commandOperation names one command operation: the operation it is, the path its fingerprint is taken
// over, the status it answers a domain refusal with, and every status it answers at all. Success and
// refusal share the declaration, because an operation answers a status with one shape whichever path
// produced it.
//
// Each operation declares its own generated response types, so a replayed answer is spelled as the
// operation that was asked rather than as one of its siblings.
type commandOperation struct {
	name string
	path string

	refused int
	answers map[int]answerShape
}

// answerOf spells what a command decided as the response object its operation declares. A status the
// operation does not declare is a defect of this server rather than a state a client can cause, so it
// reaches the client as an internal failure through the strict layer.
func answerOf(operation commandOperation, answered rentals.Answered) (any, error) {
	return operation.spell(answered.Status, answered.Body, answerHeaders{replayed: answered.Replayed})
}

// spell renders one body for one status of an operation.
func (operation commandOperation) spell(status int, encoded []byte, headers answerHeaders) (any, error) {
	shape, declared := operation.answers[status]
	if !declared {
		return nil, undeclaredAnswer(operation.name, status)
	}
	return shape(encoded, headers)
}

// rideRender spells what a ride command decided: the rental as it now stands on a move, and the refusal
// it decided on instead — spelled as the operation that was asked, because each of the three declares
// the statuses it answers. A command that decided nothing is the answer a refusal-only render would give
// for a success as well, so the branch is here rather than left to every caller.
func rideRender(ctx context.Context, operation commandOperation) rentals.Render {
	return func(outcome rentals.Outcome) (rentals.Response, error) {
		if outcome.Refused() {
			return refusalRender(ctx, operation, outcome.Refusal)
		}
		rental, err := rentalBody(outcome.Rental, outcome.Vehicle, outcome.Moment, outcome.Progress)
		if err != nil {
			return rentals.Response{}, err
		}
		return encoded(http.StatusOK, servedapi.RentalCommandResult{
			ServerTime: timestamp.Format(outcome.Moment),
			Rental:     rental,
		})
	}
}

// attemptOf describes one attempt at this command: the key a repeat answers with, the fingerprint of
// what it asked for — the method, the path with the rental it names and the body it does not take —
// and the way its answer is spelled.
func (operation commandOperation) attemptOf(
	rentalID string, key idempotency.Key, render rentals.Render,
) (rentals.Attempt, error) {
	return commandAttempt(key, operation.route(rentalID), absentBody, render)
}

// attempt describes one attempt at a ride command, which every one of them spells as itself.
func (operation commandOperation) attempt(
	ctx context.Context, rentalID string, key idempotency.Key,
) (rentals.Attempt, error) {
	return operation.attemptOf(rentalID, key, rideRender(ctx, operation))
}

// route names the resource this command acts on, which is the path its fingerprint covers.
func (operation commandOperation) route(rentalID string) string {
	return strings.Replace(operation.path, rentalPathPlaceholder, rentalID, 1)
}

// rentalPathPlaceholder is where the identifier of the rental a command names stands in its path.
const rentalPathPlaceholder = "{id}"

// refusalRender spells one refusal, carrying the identifier of the request that met it when the
// boundary has one.
func refusalRender(
	ctx context.Context, operation commandOperation, refusal rentals.Refusal,
) (rentals.Response, error) {
	code, status, message, err := refusalContract(refusal, publicRefusals)
	if err != nil {
		return rentals.Response{}, err
	}
	if _, declared := operation.answers[status]; !declared {
		return rentals.Response{}, undeclaredAnswer(operation.name, status)
	}
	details, err := refusalDetails(refusal)
	if err != nil {
		return rentals.Response{}, err
	}
	body := servedapi.ApiError{Code: code, Message: message, Details: details}
	if ctx != nil {
		body.RequestId = middleware.GetReqID(ctx)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return rentals.Response{}, err
	}
	return rentals.Response{Status: status, Body: encoded}, nil
}

// spellFailure spells a command that could not be decided as the shape its operation declares for it
// after the reported failure rather than after a stored answer.
func spellFailure(operation commandOperation, ctx context.Context, failure commandFailure) (any, error) {
	body := apiErrorBody(ctx, failure.code, failure.message)
	status := operation.refused
	if failure.code == codeServiceUnavailable {
		status = http.StatusServiceUnavailable
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return operation.spell(status, encoded, answerHeaders{retryAfter: failure.retryAfter})
}

// undeclaredAnswer reports an answer the operation does not declare.
func undeclaredAnswer(operation string, status int) error {
	return fmt.Errorf("the %s operation answered status %d, which it does not declare", operation, status)
}

func commandAttempt(key idempotency.Key, path string, body any, render rentals.Render) (rentals.Attempt, error) {
	fingerprint, err := commandFingerprintOf(http.MethodPost, path, body)
	if err != nil {
		return rentals.Attempt{}, err
	}
	return rentals.Attempt{Key: key, Fingerprint: fingerprint, Render: render}, nil
}
