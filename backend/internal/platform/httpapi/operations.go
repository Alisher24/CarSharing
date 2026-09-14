package httpapi

import (
	"net/http"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
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
			http.StatusCreated:            shapeOf(declaredBody[servedapi.ReserveResult](), reserveCreated),
			http.StatusConflict:           shapeOf(declaredBody[servedapi.ApiError](), reserveConflict),
			http.StatusServiceUnavailable: shapeOf(declaredBody[servedapi.ApiError](), reserveUnavailable),
		},
	}

	cancelOperation = commandOperation{
		name:    "cancellation",
		path:    reservePath + "/{id}" + cancelRentalSuffix,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(declaredBody[servedapi.RentalCommandResult](), cancelSucceeded),
			http.StatusNotFound:           shapeOf(declaredBody[servedapi.ApiError](), cancelNotFound),
			http.StatusConflict:           shapeOf(declaredBody[servedapi.ApiError](), cancelConflict),
			http.StatusServiceUnavailable: shapeOf(declaredBody[servedapi.ApiError](), cancelUnavailable),
		},
	}

	startRideOperation = commandOperation{
		name:    "start",
		path:    startRentalPath,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(declaredBody[servedapi.RentalCommandResult](), startSucceeded),
			http.StatusNotFound:           shapeOf(declaredBody[servedapi.ApiError](), startNotFound),
			http.StatusConflict:           shapeOf(declaredBody[servedapi.ApiError](), startConflict),
			http.StatusServiceUnavailable: shapeOf(declaredBody[servedapi.ApiError](), startUnavailable),
		},
	}

	pauseRideOperation = commandOperation{
		name:    "pause",
		path:    pauseRentalPath,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(declaredBody[servedapi.RentalCommandResult](), pauseSucceeded),
			http.StatusNotFound:           shapeOf(declaredBody[servedapi.ApiError](), pauseNotFound),
			http.StatusConflict:           shapeOf(declaredBody[servedapi.ApiError](), pauseConflict),
			http.StatusServiceUnavailable: shapeOf(declaredBody[servedapi.ApiError](), pauseUnavailable),
		},
	}

	resumeRideOperation = commandOperation{
		name:    "resume",
		path:    resumeRentalPath,
		refused: http.StatusConflict,
		answers: map[int]answerShape{
			http.StatusOK:                 shapeOf(declaredBody[servedapi.RentalCommandResult](), resumeSucceeded),
			http.StatusNotFound:           shapeOf(declaredBody[servedapi.ApiError](), resumeNotFound),
			http.StatusConflict:           shapeOf(declaredBody[servedapi.ApiError](), resumeConflict),
			http.StatusServiceUnavailable: shapeOf(declaredBody[servedapi.ApiError](), resumeUnavailable),
		},
	}
)

// reserveCreated spells the answer of a reservation that was made.
func reserveCreated(body servedapi.ReserveResult, _ bool) any {
	return servedapi.Reserve201JSONResponse{
		Body:    body,
		Headers: servedapi.Reserve201ResponseHeaders{},
	}
}

// reserveConflict spells a reservation that was refused.
func reserveConflict(body servedapi.ApiError, _ bool) any {
	return servedapi.Reserve409JSONResponse{
		Body:    body,
		Headers: servedapi.Reserve409ResponseHeaders{RetryAfter: retryAfterOf(body)},
	}
}

// cancelSucceeded spells the answer of a reservation that was given back.
func cancelSucceeded(body servedapi.RentalCommandResult, _ bool) any {
	return servedapi.CancelRental200JSONResponse{
		Body:    body,
		Headers: servedapi.CancelRental200ResponseHeaders{},
	}
}

// cancelNotFound spells a rental this account does not hold, which is the answer to a cancellation and
// to a ride command alike: a missing rental and somebody else's are one answer.
func cancelNotFound(body servedapi.ApiError, _ bool) any {
	return servedapi.CancelRental404JSONResponse{Body: body}
}

// cancelConflict spells a cancellation that was refused.
func cancelConflict(body servedapi.ApiError, _ bool) any {
	return servedapi.CancelRental409JSONResponse{
		Body:    body,
		Headers: servedapi.CancelRental409ResponseHeaders{RetryAfter: retryAfterOf(body)},
	}
}

// startSucceeded, pauseSucceeded and resumeSucceeded spell the answer of a ride the command moved.
func startSucceeded(body servedapi.RentalCommandResult, _ bool) any {
	return servedapi.StartRental200JSONResponse{
		Body:    body,
		Headers: servedapi.StartRental200ResponseHeaders{},
	}
}

func pauseSucceeded(body servedapi.RentalCommandResult, _ bool) any {
	return servedapi.PauseRental200JSONResponse{
		Body:    body,
		Headers: servedapi.PauseRental200ResponseHeaders{},
	}
}

func resumeSucceeded(body servedapi.RentalCommandResult, _ bool) any {
	return servedapi.ResumeRental200JSONResponse{
		Body:    body,
		Headers: servedapi.ResumeRental200ResponseHeaders{},
	}
}

// startNotFound, pauseNotFound and resumeNotFound spell a rental this account does not hold.
func startNotFound(body servedapi.ApiError, _ bool) any {
	return servedapi.StartRental404JSONResponse{Body: body}
}

func pauseNotFound(body servedapi.ApiError, _ bool) any {
	return servedapi.PauseRental404JSONResponse{Body: body}
}

func resumeNotFound(body servedapi.ApiError, _ bool) any {
	return servedapi.ResumeRental404JSONResponse{Body: body}
}

// startConflict, pauseConflict and resumeConflict spell a ride command that was refused.
func startConflict(body servedapi.ApiError, _ bool) any {
	return servedapi.StartRental409JSONResponse{
		Body:    body,
		Headers: servedapi.StartRental409ResponseHeaders{RetryAfter: retryAfterOf(body)},
	}
}

func pauseConflict(body servedapi.ApiError, _ bool) any {
	return servedapi.PauseRental409JSONResponse{
		Body:    body,
		Headers: servedapi.PauseRental409ResponseHeaders{RetryAfter: retryAfterOf(body)},
	}
}

func resumeConflict(body servedapi.ApiError, _ bool) any {
	return servedapi.ResumeRental409JSONResponse{
		Body:    body,
		Headers: servedapi.ResumeRental409ResponseHeaders{RetryAfter: retryAfterOf(body)},
	}
}

// reserveUnavailable, cancelUnavailable, startUnavailable, pauseUnavailable and resumeUnavailable spell
// a command that could not be decided at all, which is the one answer every command operation states
// for the same reason: the process, not the request, is what failed.
func reserveUnavailable(body servedapi.ApiError, _ bool) any {
	return servedapi.Reserve503JSONResponse{Body: body}
}

func cancelUnavailable(body servedapi.ApiError, _ bool) any {
	return servedapi.CancelRental503JSONResponse{Body: body}
}

func startUnavailable(body servedapi.ApiError, _ bool) any {
	return servedapi.StartRental503JSONResponse{Body: body}
}

func pauseUnavailable(body servedapi.ApiError, _ bool) any {
	return servedapi.PauseRental503JSONResponse{Body: body}
}

func resumeUnavailable(body servedapi.ApiError, _ bool) any {
	return servedapi.ResumeRental503JSONResponse{Body: body}
}

// markReplayed marks an answer an earlier attempt already gave. It is a header on the response object
// rather than a status, and every command operation declares it for every status it answers.
func markReplayed(spelled any) any {
	switch answer := spelled.(type) {
	case servedapi.Reserve201JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	case servedapi.Reserve409JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	case servedapi.CancelRental200JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	case servedapi.CancelRental409JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	case servedapi.StartRental200JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	case servedapi.StartRental409JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	case servedapi.PauseRental200JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	case servedapi.PauseRental409JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	case servedapi.ResumeRental200JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	case servedapi.ResumeRental409JSONResponse:
		answer.Headers.IdempotencyReplayed = replayedHeader(true)
		return answer
	default:
		return spelled
	}
}

// attachRetryAfter asks for a wait when a failure carries one. A stored refusal that carries a moment
// of its own — an exhausted allowance above all — never borrows this header for it.
func attachRetryAfter(spelled any, retryAfter *int) any {
	if retryAfter == nil {
		return spelled
	}
	switch answer := spelled.(type) {
	case servedapi.Reserve409JSONResponse:
		answer.Headers.RetryAfter = retryAfter
		return answer
	case servedapi.CancelRental409JSONResponse:
		answer.Headers.RetryAfter = retryAfter
		return answer
	case servedapi.StartRental409JSONResponse:
		answer.Headers.RetryAfter = retryAfter
		return answer
	case servedapi.PauseRental409JSONResponse:
		answer.Headers.RetryAfter = retryAfter
		return answer
	case servedapi.ResumeRental409JSONResponse:
		answer.Headers.RetryAfter = retryAfter
		return answer
	default:
		return spelled
	}
}
