package httpapi

import (
	"net/http"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// The operations that answer one command. A reservation is made and given back, and a ride is started,
// paused and continued: each declares every status the contract states for it, with the operation's own
// generated response types, so a replayed answer is spelled as the operation that was asked and a new
// command is a new declaration rather than a new branch in a growing conditional.
var (
	reserveOperation = commandOperation{
		name:    "reservation",
		path:    reservePath,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusCreated:            shapeOf(reserveCreated),
			http.StatusConflict:           shapeOf(reserveConflict),
			http.StatusServiceUnavailable: shapeOf(reserveUnavailable),
		},
	}

	cancelOperation = commandOperation{
		name:    "cancellation",
		path:    reservePath + "/{id}" + cancelRentalSuffix,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(cancelSucceeded),
			http.StatusNotFound:           shapeOf(cancelNotFound),
			http.StatusConflict:           shapeOf(cancelConflict),
			http.StatusServiceUnavailable: shapeOf(cancelUnavailable),
		},
	}

	startRideOperation = commandOperation{
		name:    string(rentals.StartRental),
		path:    startRentalPath,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(startSucceeded),
			http.StatusNotFound:           shapeOf(startNotFound),
			http.StatusConflict:           shapeOf(startConflict),
			http.StatusServiceUnavailable: shapeOf(startUnavailable),
		},
	}

	pauseRideOperation = commandOperation{
		name:    string(rentals.PauseRental),
		path:    pauseRentalPath,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(pauseSucceeded),
			http.StatusNotFound:           shapeOf(pauseNotFound),
			http.StatusConflict:           shapeOf(pauseConflict),
			http.StatusServiceUnavailable: shapeOf(pauseUnavailable),
		},
	}

	resumeRideOperation = commandOperation{
		name:    string(rentals.ResumeRental),
		path:    resumeRentalPath,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(resumeSucceeded),
			http.StatusNotFound:           shapeOf(resumeNotFound),
			http.StatusConflict:           shapeOf(resumeConflict),
			http.StatusServiceUnavailable: shapeOf(resumeUnavailable),
		},
	}

	// A finish answers a different shape from the three commands that move a ride: the ride it ended
	// together with the invoice of it. Its statuses are the ones the contract declares for it, which
	// are the same set the ride commands state.
	finishRentalOperation = commandOperation{
		name:    "finish",
		path:    finishRentalPath,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(finishSucceeded),
			http.StatusNotFound:           shapeOf(finishNotFound),
			http.StatusConflict:           shapeOf(finishConflict),
			http.StatusServiceUnavailable: shapeOf(finishUnavailable),
		},
	}

	// A payment answers the invoice it settled rather than the ride it belongs to, which is the whole
	// of what a person who asked to pay is told. It states the same statuses the finish does, because
	// the refusals a payment meets are the ones a missing invoice and a payment in flight carry.
	payInvoiceOperation = commandOperation{
		name:    "payment",
		path:    payInvoicePath,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(paySucceeded),
			http.StatusNotFound:           shapeOf(payNotFound),
			http.StatusConflict:           shapeOf(payConflict),
			http.StatusServiceUnavailable: shapeOf(payUnavailable),
		},
	}
)

// reserveCreated spells the answer of a reservation that was made.
func reserveCreated(body servedapi.ReserveResult, headers answerHeaders) any {
	return servedapi.Reserve201JSONResponse{
		Body:    body,
		Headers: servedapi.Reserve201ResponseHeaders{IdempotencyReplayed: replayedHeader(headers.replayed)},
	}
}

// reserveConflict spells a reservation that was refused.
func reserveConflict(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.Reserve409JSONResponse{
		Body: body,
		Headers: servedapi.Reserve409ResponseHeaders{
			IdempotencyReplayed: replayedHeader(headers.replayed),
			RetryAfter:          headers.retry(body.Code),
		},
	}
}

// cancelSucceeded spells the answer of a reservation that was given back.
func cancelSucceeded(body servedapi.RentalCommandResult, headers answerHeaders) any {
	return servedapi.CancelRental200JSONResponse{
		Body:    body,
		Headers: servedapi.CancelRental200ResponseHeaders{IdempotencyReplayed: replayedHeader(headers.replayed)},
	}
}

// cancelNotFound spells a rental this account does not hold, which is the answer to a cancellation and
// to a ride command alike: a missing rental and somebody else's are one answer.
func cancelNotFound(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.CancelRental404JSONResponse{Body: body}
}

// cancelConflict spells a cancellation that was refused.
func cancelConflict(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.CancelRental409JSONResponse{
		Body: body,
		Headers: servedapi.CancelRental409ResponseHeaders{
			IdempotencyReplayed: replayedHeader(headers.replayed),
			RetryAfter:          headers.retry(body.Code),
		},
	}
}

// startSucceeded, pauseSucceeded and resumeSucceeded spell the answer of a ride the command moved.
func startSucceeded(body servedapi.RentalCommandResult, headers answerHeaders) any {
	return servedapi.StartRental200JSONResponse{
		Body:    body,
		Headers: servedapi.StartRental200ResponseHeaders{IdempotencyReplayed: replayedHeader(headers.replayed)},
	}
}

func pauseSucceeded(body servedapi.RentalCommandResult, headers answerHeaders) any {
	return servedapi.PauseRental200JSONResponse{
		Body:    body,
		Headers: servedapi.PauseRental200ResponseHeaders{IdempotencyReplayed: replayedHeader(headers.replayed)},
	}
}

func resumeSucceeded(body servedapi.RentalCommandResult, headers answerHeaders) any {
	return servedapi.ResumeRental200JSONResponse{
		Body:    body,
		Headers: servedapi.ResumeRental200ResponseHeaders{IdempotencyReplayed: replayedHeader(headers.replayed)},
	}
}

// startNotFound, pauseNotFound and resumeNotFound spell a rental this account does not hold.
func startNotFound(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.StartRental404JSONResponse{Body: body}
}

func pauseNotFound(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.PauseRental404JSONResponse{Body: body}
}

func resumeNotFound(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.ResumeRental404JSONResponse{Body: body}
}

// startConflict, pauseConflict and resumeConflict spell a ride command that was refused.
func startConflict(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.StartRental409JSONResponse{
		Body: body,
		Headers: servedapi.StartRental409ResponseHeaders{
			IdempotencyReplayed: replayedHeader(headers.replayed),
			RetryAfter:          headers.retry(body.Code),
		},
	}
}

func pauseConflict(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.PauseRental409JSONResponse{
		Body: body,
		Headers: servedapi.RideMoveConflictResponseHeaders{
			IdempotencyReplayed: replayedHeader(headers.replayed),
			RetryAfter:          headers.retry(body.Code),
		},
	}
}

func resumeConflict(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.ResumeRental409JSONResponse{
		Body: body,
		Headers: servedapi.RideMoveConflictResponseHeaders{
			IdempotencyReplayed: replayedHeader(headers.replayed),
			RetryAfter:          headers.retry(body.Code),
		},
	}
}

// reserveUnavailable, cancelUnavailable, startUnavailable, pauseUnavailable and resumeUnavailable spell
// a command that could not be decided at all, which is the one answer every command operation states
// for the same reason: the process, not the request, is what failed.
func reserveUnavailable(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.Reserve503JSONResponse{Body: body}
}

func cancelUnavailable(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.CancelRental503JSONResponse{Body: body}
}

func startUnavailable(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.StartRental503JSONResponse{Body: body}
}

func pauseUnavailable(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.PauseRental503JSONResponse{Body: body}
}

func resumeUnavailable(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.ResumeRental503JSONResponse{Body: body}
}

// finishSucceeded spells the answer of a ride that ended, which carries the invoice of it.
func finishSucceeded(body servedapi.FinishResult, headers answerHeaders) any {
	return servedapi.FinishRental200JSONResponse{
		Body:    body,
		Headers: servedapi.FinishRental200ResponseHeaders{IdempotencyReplayed: replayedHeader(headers.replayed)},
	}
}

// finishNotFound spells a rental this account does not hold.
func finishNotFound(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.FinishRental404JSONResponse{Body: body}
}

// finishConflict spells a finish that was refused where the ride stands or how its position reads.
func finishConflict(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.FinishRental409JSONResponse{
		Body: body,
		Headers: servedapi.FinishRental409ResponseHeaders{
			IdempotencyReplayed: replayedHeader(headers.replayed),
			RetryAfter:          headers.retry(body.Code),
		},
	}
}

// finishUnavailable spells a finish that could not be decided at all.
func finishUnavailable(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.FinishRental503JSONResponse{Body: body}
}

// paySucceeded spells the answer of an invoice whose payment was decided, which carries the invoice
// with the state of its payment rather than the ride it belongs to.
func paySucceeded(body servedapi.PayResult, headers answerHeaders) any {
	return servedapi.PayInvoice200JSONResponse{
		Body:    body,
		Headers: servedapi.PayInvoice200ResponseHeaders{IdempotencyReplayed: replayedHeader(headers.replayed)},
	}
}

// payNotFound spells an invoice this account does not hold.
func payNotFound(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.PayInvoice404JSONResponse{Body: body}
}

// payConflict spells a payment that was refused: an invoice whose first attempt the service still owes,
// and the two answers a command key that is already in use is given.
func payConflict(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.PayInvoice409JSONResponse{
		Body: body,
		Headers: servedapi.PayInvoice409ResponseHeaders{
			IdempotencyReplayed: replayedHeader(headers.replayed),
			RetryAfter:          headers.retry(body.Code),
		},
	}
}

// payUnavailable spells a payment that could not be decided at all.
func payUnavailable(body servedapi.ApiError, headers answerHeaders) any {
	return servedapi.PayInvoice503JSONResponse{Body: body}
}
