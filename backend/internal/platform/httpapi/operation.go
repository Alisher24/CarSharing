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

// decodeBody turns one stored answer into the contract type its operation publishes, so a repeated
// command is answered with what the first attempt answered rather than with a body assembled a second
// time.
type decodeBody[T any] func([]byte) (T, error)

func declaredBody[T any]() decodeBody[T] {
	return func(stored []byte) (T, error) {
		var body T
		err := json.Unmarshal(stored, &body)
		return body, err
	}
}

// spellResponse wraps one decoded body as the generated response object one operation declares for one
// status. Which attempt produced the answer is a header rather than a status, so it is passed here,
// and the operation's own response type is what carries it.
type spellResponse[T any] func(T, bool) any

// answerShape is how one status of one operation is spelled: the operation's own response type, filled
// from the body the contract publishes for that status.
type answerShape struct {
	decode func([]byte) (any, error)
}

func shapeOf[T any](body decodeBody[T], spell spellResponse[T]) answerShape {
	return answerShape{
		decode: func(stored []byte) (any, error) {
			decoded, err := body(stored)
			if err != nil {
				return nil, err
			}
			return spell(decoded, false), nil
		},
	}
}

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
	return operation.spell(answered.Status, answered.Body, answered.Replayed)
}

// spell renders one body for one status of an operation.
func (operation commandOperation) spell(status int, encoded []byte, replayed bool) (any, error) {
	shape, declared := operation.answers[status]
	if !declared {
		return nil, undeclaredAnswer(operation.name, status)
	}
	spelled, err := shape.decode(encoded)
	if err != nil {
		return nil, err
	}
	if replayed {
		spelled = markReplayed(spelled)
	}
	return spelled, nil
}

// rideRender spells what a ride command decided: the rental as it now stands on a move, and the refusal
// it decided on instead. A command that decided nothing is the answer a refusal-only render would give
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

// attempt describes one attempt at this command: the key a repeat answers with, the fingerprint of what
// it asked for, which is the method, the path with the rental it names and the body it does not take,
// and the way its answer is spelled.
func (operation commandOperation) attempt(
	ctx context.Context, rentalID string, key idempotency.Key,
) (rentals.Attempt, error) {
	fingerprint, err := commandFingerprintOf(http.MethodPost, operation.route(rentalID), absentBody)
	if err != nil {
		return rentals.Attempt{}, err
	}
	return rentals.Attempt{
		Key:         key,
		Fingerprint: fingerprint,
		Render:      rideRender(ctx, operation),
	}, nil
}

// route names the resource this command acts on, which is the path its fingerprint covers.
func (operation commandOperation) route(rentalID string) string {
	return strings.Replace(operation.path, rentalPathPlaceholder, rentalID, 1)
}

// rentalPathPlaceholder is where the identifier of the rental a command names stands in its path.
const rentalPathPlaceholder = "{id}"

// refusalRender spells a domain refusal as the answer its operation declares. The code carries its own
// status, and the details a client displays travel in the error envelope rather than beside it.
func (operation commandOperation) refusalRender(outcome rentals.Outcome) (rentals.Response, error) {
	return refusalRender(nil, operation, outcome.Refusal)
}

// refusalRender spells one refusal, carrying the identifier of the request that met it when the
// boundary has one.
func refusalRender(
	ctx context.Context, operation commandOperation, refusal rentals.Refusal,
) (rentals.Response, error) {
	code, status, message, err := refusalContract(refusal)
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
	spelled, err := operation.spell(status, encoded, false)
	if err != nil {
		return nil, err
	}
	return attachRetryAfter(spelled, failure.retryAfter), nil
}

// undeclaredAnswer reports an answer the operation does not declare.
func undeclaredAnswer(operation string, status int) error {
	return fmt.Errorf("the %s operation answered status %d, which it does not declare", operation, status)
}
