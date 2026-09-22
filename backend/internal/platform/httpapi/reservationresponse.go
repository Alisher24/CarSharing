package httpapi

import (
	"context"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// reserveAnswer returns the answer of a reservation command as its operation declares it.
func reserveAnswer(answered rentals.Answered) (servedapi.ReserveResponseObject, error) {
	spelled, err := answerOf(reserveOperation, answered)
	if err != nil {
		return nil, err
	}
	return spelled.(servedapi.ReserveResponseObject), nil
}

// cancelAnswer returns the answer of a cancellation as its operation declares it.
func cancelAnswer(answered rentals.Answered) (servedapi.CancelRentalResponseObject, error) {
	spelled, err := answerOf(cancelOperation, answered)
	if err != nil {
		return nil, err
	}
	return spelled.(servedapi.CancelRentalResponseObject), nil
}

// reserveFailure answers a reservation command that could not be decided.
func reserveFailure(ctx context.Context, failure commandFailure) servedapi.ReserveResponseObject {
	spelled, _ := spellFailure(reserveOperation, ctx, failure)
	return spelled.(servedapi.ReserveResponseObject)
}

// cancelFailure answers a cancellation that could not be decided.
func cancelFailure(ctx context.Context, failure commandFailure) servedapi.CancelRentalResponseObject {
	spelled, _ := spellFailure(cancelOperation, ctx, failure)
	return spelled.(servedapi.CancelRentalResponseObject)
}

// retryAfterOf asks for a wait when the answer asks for one. Two answers do, for the same reason and
// with the same interval: a command whose twin is still running, and a payment whose attempt the
// service has not finished making. A stored refusal that carries a moment of its own — an exhausted
// allowance above all — never borrows this header for it.
func retryAfterOf(code servedapi.ErrorCode) *int {
	if !commandWaits[code] {
		return nil
	}
	return retryAfterCommandBusy()
}

var commandWaits = map[servedapi.ErrorCode]bool{
	servedapi.IDEMPOTENCYINPROGRESS: true,
	servedapi.PAYMENTINPROGRESS:     true,
}
