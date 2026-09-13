package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// reserveAnswer returns the answer of a reservation command as its operation declares it. A stored
// answer is decoded back into the contract's own type, so a repeated command is answered with what
// the first attempt answered rather than with a body assembled a second time.
func reserveAnswer(answered rentals.Answered) (servedapi.ReserveResponseObject, error) {
	switch answered.Status {
	case http.StatusCreated:
		body, err := decodedBody[servedapi.ReserveResult](answered)
		if err != nil {
			return nil, err
		}
		return servedapi.Reserve201JSONResponse{
			Body: body,
			Headers: servedapi.Reserve201ResponseHeaders{
				IdempotencyReplayed: replayedHeader(answered.Replayed),
			},
		}, nil
	case http.StatusConflict:
		body, err := decodedBody[servedapi.ApiError](answered)
		if err != nil {
			return nil, err
		}
		return servedapi.Reserve409JSONResponse{
			Body: body,
			Headers: servedapi.Reserve409ResponseHeaders{
				IdempotencyReplayed: replayedHeader(answered.Replayed),
				RetryAfter:          retryAfterOf(body),
			},
		}, nil
	default:
		return nil, undeclaredAnswer("reservation", answered.Status)
	}
}

// cancelAnswer returns the answer of a cancellation as its operation declares it.
func cancelAnswer(answered rentals.Answered) (servedapi.CancelRentalResponseObject, error) {
	switch answered.Status {
	case http.StatusOK:
		body, err := decodedBody[servedapi.RentalCommandResult](answered)
		if err != nil {
			return nil, err
		}
		return servedapi.CancelRental200JSONResponse{
			Body: body,
			Headers: servedapi.CancelRental200ResponseHeaders{
				IdempotencyReplayed: replayedHeader(answered.Replayed),
			},
		}, nil
	case http.StatusNotFound:
		body, err := decodedBody[servedapi.ApiError](answered)
		if err != nil {
			return nil, err
		}
		return servedapi.CancelRental404JSONResponse{Body: body}, nil
	case http.StatusConflict:
		body, err := decodedBody[servedapi.ApiError](answered)
		if err != nil {
			return nil, err
		}
		return servedapi.CancelRental409JSONResponse{
			Body: body,
			Headers: servedapi.CancelRental409ResponseHeaders{
				IdempotencyReplayed: replayedHeader(answered.Replayed),
				RetryAfter:          retryAfterOf(body),
			},
		}, nil
	default:
		return nil, undeclaredAnswer("cancellation", answered.Status)
	}
}

// reserveFailure answers a reservation command that could not be decided.
func reserveFailure(ctx context.Context, failure commandFailure) servedapi.ReserveResponseObject {
	body := apiErrorBody(ctx, failure.code, failure.message)
	if failure.code == codeServiceUnavailable {
		return servedapi.Reserve503JSONResponse{Body: body}
	}
	return servedapi.Reserve409JSONResponse{
		Body:    body,
		Headers: servedapi.Reserve409ResponseHeaders{RetryAfter: failure.retryAfter},
	}
}

// cancelFailure answers a cancellation that could not be decided.
func cancelFailure(ctx context.Context, failure commandFailure) servedapi.CancelRentalResponseObject {
	body := apiErrorBody(ctx, failure.code, failure.message)
	if failure.code == codeServiceUnavailable {
		return servedapi.CancelRental503JSONResponse{Body: body}
	}
	return servedapi.CancelRental409JSONResponse{
		Body:    body,
		Headers: servedapi.CancelRental409ResponseHeaders{RetryAfter: failure.retryAfter},
	}
}

// retryAfterOf asks for a wait only when the answer says the same command is still running. A stored
// refusal that carries a moment of its own — an exhausted allowance above all — never borrows this
// header for it.
func retryAfterOf(body servedapi.ApiError) *int {
	if body.Code != servedapi.IDEMPOTENCYINPROGRESS {
		return nil
	}
	return retryAfterCommandBusy()
}

func decodedBody[T any](answered rentals.Answered) (T, error) {
	var body T
	err := json.Unmarshal(answered.Body, &body)
	return body, err
}

// undeclaredAnswer reports an answer the operation does not declare. It is a defect of this server
// rather than a state a client can cause, so it reaches the client as an internal failure through
// the strict layer rather than as a status the contract never mentions.
func undeclaredAnswer(operation string, status int) error {
	return fmt.Errorf("a %s command answered status %d, which its operation does not declare",
		operation, status)
}
