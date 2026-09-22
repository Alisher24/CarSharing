package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/google/uuid"
)

// The paths of the operations that require a command key. They are the specification's own routes,
// stated once here because the fingerprint of a command covers the path including the identifier of
// the resource it names.
const (
	reservePath        = "/api/v1/reservations"
	cancelRentalSuffix = "/cancel"
)

// commandKeyHeader returns the key a mutating command was sent with. The validation boundary has
// already refused a missing or malformed key, so the value is a canonical UUID v4.
func commandKeyHeader(params servedapi.CommandId) idempotency.Key {
	return idempotency.Key(params)
}

// commandFingerprintOf reduces one command to the fingerprint a repeat must match. The body is the
// contract's own shape for it, re-encoded here, so two attempts that ask for the same thing
// fingerprint alike however they were written.
func commandFingerprintOf(method, path string, body any) (idempotency.Fingerprint, error) {
	canonical, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return idempotency.FingerprintOf(method, path, canonical), nil
}

// commandFailure is the answer a command failure leaves: the transport code, its message and the
// wait it asks for. A domain refusal never arrives here, because it is an answer the module decided
// on rather than a failure of the request.
type commandFailure struct {
	code       servedapi.ErrorCode
	message    string
	retryAfter *int
}

// commandFailureOf translates a failure of the rentals module. Every case is repeatable: the same
// command with the same key gets the same class of answer again, and none of them reports a change that
// did not happen.
func commandFailureOf(err error) commandFailure {
	switch {
	case errors.Is(err, idempotency.ErrFingerprintMismatch):
		return commandFailure{
			code:    servedapi.IDEMPOTENCYCONFLICT,
			message: messageIdempotencyConflict,
		}
	case errors.Is(err, idempotency.ErrInProgress):
		return commandFailure{
			code:       servedapi.IDEMPOTENCYINPROGRESS,
			message:    messageIdempotencyInProgress,
			retryAfter: retryAfterCommandBusy(),
		}
	case errors.Is(err, rentals.ErrConcurrencyExhausted):
		return commandFailure{code: codeServiceUnavailable, message: messageServiceUnavailable}
	default:
		return commandFailure{code: codeServiceUnavailable, message: messageServiceUnavailable}
	}
}

// reportUncarried records a failure the client is told only as a service failure. A command that
// could not be decided is a defect or an outage rather than an answer, so the process says which.
func reportUncarried(ctx context.Context, err error, failure commandFailure) {
	if failure.code == codeServiceUnavailable {
		slog.ErrorContext(ctx, "a rental command could not be decided", "error", err)
	}
}

// retryAfterCommandBusy is the wait an unfinished command asks for. It is one second because the
// attempt it waits for is a short transaction, and it is deliberately not the moment the daily
// allowance returns: that moment is the one the refusal itself carries.
func retryAfterCommandBusy() *int {
	const seconds = 1
	return ptr(seconds)
}

func ptr[T any](value T) *T { return &value }

// replayedHeader marks an answer that an earlier attempt already gave. It is absent on a fresh
// answer, which is how a client tells a repeat from a command that has just happened.
func replayedHeader(replayed bool) *bool {
	if !replayed {
		return nil
	}
	return ptr(true)
}

// Reservations is what the operations on one's own rentals need from the rentals module: the
// commands that move a rental, the read that answers what is current, and the page of the ones that
// have finished.
type Reservations interface {
	Reserve(ctx context.Context, command rentals.ReserveCommand) (rentals.Answered, error)
	Current(ctx context.Context, caller uuid.UUID) (rentals.Current, error)
	Rides(ctx context.Context, caller uuid.UUID, after *cursor.Position, limit int) (rentals.RidePage, error)

	// Apply carries a command that names one of the caller's rentals — starting, pausing and
	// continuing a ride, and giving a reservation back — and the finish ends a ride and issues its
	// invoice. They belong to the same module and the same surface, so they are asked of the same
	// dependency rather than of a second one naming one implementation twice.
	Apply(ctx context.Context, command rentals.RentalCommand) (rentals.Answered, error)
	Finish(ctx context.Context, command rentals.FinishCommand) (rentals.Answered, error)
	Pay(ctx context.Context, command rentals.PayCommand) (rentals.Answered, error)
}

// reservationHandlers answers the operations that belong to one signed-in person's own reservation.
type reservationHandlers struct{ reservations Reservations }

func newReservationHandlers(reservations Reservations) (reservationHandlers, error) {
	if reservations == nil {
		return reservationHandlers{}, fmt.Errorf("%w: reservation commands", ErrIncompleteApplication)
	}
	return reservationHandlers{reservations: reservations}, nil
}

// callerOf resolves the account a request belongs to. The boundary has already refused a request
// without a live session, so an unresolved account here is a session that ended between the two
// checks, which is answered the same way as one that never existed.
func callerOf(ctx context.Context) (uuid.UUID, bool) {
	snapshot, live, err := sessionOf(ctx).resolve(ctx)
	if err != nil || !live {
		return uuid.Nil, false
	}
	return snapshot.UserID, true
}

// GetCurrentRental answers what the caller is doing now. A read that could not record a transition
// it discovered is reported as a failure rather than as an absence: a client must not be told that
// its vehicle is free when the release could not be written.
func (h reservationHandlers) GetCurrentRental(
	ctx context.Context, _ servedapi.GetCurrentRentalRequestObject,
) (servedapi.GetCurrentRentalResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.GetCurrentRental401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}

	current, err := h.reservations.Current(ctx, caller)
	if err != nil {
		return servedapi.GetCurrentRental503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	body, err := currentSnapshot(current)
	if err != nil {
		return nil, err
	}
	return servedapi.GetCurrentRental200JSONResponse{Body: body}, nil
}

// Reserve holds one vehicle for the caller. Everything the command acts on is read from storage
// under the transaction's own locks: no field of the request selects the owner, the moment, the
// state or the rates.
func (h reservationHandlers) Reserve(
	ctx context.Context, request servedapi.ReserveRequestObject,
) (servedapi.ReserveResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.Reserve401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	if request.Body == nil {
		return nil, errors.New("a reservation request arrived without a body")
	}
	fingerprint, err := commandFingerprintOf(http.MethodPost, reservePath, *request.Body)
	if err != nil {
		return nil, err
	}

	answered, err := h.reservations.Reserve(ctx, rentals.ReserveCommand{
		Caller:    caller,
		VehicleID: request.Body.VehicleId,
		Attempt:   reserveAttempt(ctx, commandKeyHeader(request.Params.IdempotencyKey), fingerprint),
	})
	if err != nil {
		failure := commandFailureOf(err)
		reportUncarried(ctx, err, failure)
		return reserveFailure(ctx, failure), nil
	}
	return reserveAnswer(answered)
}

// CancelRental gives the caller's own reservation back.
func (h reservationHandlers) CancelRental(
	ctx context.Context, request servedapi.CancelRentalRequestObject,
) (servedapi.CancelRentalResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.CancelRental401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	fingerprint, err := commandFingerprintOf(http.MethodPost,
		reservePath+"/"+string(request.Id)+cancelRentalSuffix, absentBody)
	if err != nil {
		return nil, err
	}

	answered, err := h.reservations.Apply(ctx, rentals.RentalCommand{
		Action:   rentals.CancelRental,
		Caller:   caller,
		RentalID: string(request.Id),
		Attempt:  cancelAttempt(ctx, commandKeyHeader(request.Params.IdempotencyKey), fingerprint),
	})
	if err != nil {
		failure := commandFailureOf(err)
		reportUncarried(ctx, err, failure)
		return cancelFailure(ctx, failure), nil
	}
	return cancelAnswer(answered)
}

// absentBody fingerprints a command that carries no body. The contract states that no request body
// is permitted on a cancellation, so every attempt at one command asks for the same thing.
var absentBody struct{}

func reserveAttempt(ctx context.Context, key idempotency.Key, fingerprint idempotency.Fingerprint) rentals.Attempt {
	return rentals.Attempt{
		Key:         key,
		Fingerprint: fingerprint,
		Render: func(outcome rentals.Outcome) (rentals.Response, error) {
			return reserveRender(ctx, outcome)
		},
	}
}

func cancelAttempt(ctx context.Context, key idempotency.Key, fingerprint idempotency.Fingerprint) rentals.Attempt {
	return rentals.Attempt{
		Key:         key,
		Fingerprint: fingerprint,
		Render: func(outcome rentals.Outcome) (rentals.Response, error) {
			return cancelRender(ctx, outcome)
		},
	}
}

func registerReservationHandlers(served *server, dependencies Dependencies) error {
	var err error
	served.reservationHandlers, err = newReservationHandlers(dependencies.Reservations)
	return err
}
