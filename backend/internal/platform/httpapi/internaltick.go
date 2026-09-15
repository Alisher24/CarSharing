package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/go-chi/chi/v5/middleware"
)

// simulationTickPath is the route of the simulator's tick. It is the specification's own path, stated
// once here so that the handler and the fingerprint of the call name the same one.
const simulationTickPath = "/internal/v1/simulation/tick"

// SimulationTick advances every simulated vehicle to one moment and answers what it changed.
func (h internalHandlers) SimulationTick(
	ctx context.Context, request internalapi.SimulationTickRequestObject,
) (internalapi.SimulationTickResponseObject, error) {
	if request.Body == nil {
		return nil, errors.New("a tick must carry the identifier it is called with")
	}
	attempt, err := internalAttemptOf(request.Body.TickId, simulationTickPath, request.Body,
		simulationTickRender())
	if err != nil {
		return nil, err
	}
	answered, err := h.simulation.Tick(ctx, rentals.TickCommand{
		TickID:  request.Body.TickId,
		Attempt: attempt,
	})
	spelled, err := tickAnswer(ctx, answered, err)
	if err != nil {
		return nil, err
	}
	return spelled.(internalapi.SimulationTickResponseObject), nil
}

// simulationTickRender spells what a tick decided: what it changed, or the refusal it decided on
// instead. A tick refuses nothing a client can cause, so a refusal of it is a defect of the module
// rather than an answer a simulator is expected to meet.
func simulationTickRender() rentals.Render {
	return func(outcome rentals.Outcome) (rentals.Response, error) {
		if outcome.Refused() {
			refused, err := refuse(outcome.Refusal)
			if err != nil {
				return rentals.Response{}, err
			}
			return rentals.Response{Status: refused.status, Body: refused.body}, nil
		}
		return encoded(http.StatusOK, internalapi.TickResult{
			TickId:             outcome.Tick.TickID,
			ServerTime:         timestamp.Format(outcome.Tick.Moment),
			ProcessedAt:        timestamp.Format(outcome.Tick.Moment),
			ChangedVehicleIds:  internalIdentifiers(outcome.Tick.ChangedVehicles),
			CompletedRentalIds: internalIdentifiers(outcome.Tick.CompletedRentals),
		})
	}
}

// tickAnswer spells what a tick decided, or the failure that kept it from being decided.
func tickAnswer(
	ctx context.Context, answered rentals.Answered, failure error,
) (any, error) {
	if failure != nil {
		return internalFailureAnswer(tickAnswers, ctx, failure)
	}
	return spellInternal(tickAnswers, answered)
}

// tickAnswers is every status the tick operation declares, each spelled as the operation's own
// response type, so an answer stored under a tick's key is published as the tick that was asked.
var tickAnswers = internalAnswers{
	http.StatusOK: internalShape(func(body internalapi.TickResult, replayed bool) any {
		return internalapi.SimulationTick200JSONResponse{
			Body: body,
			Headers: internalapi.SimulationTick200ResponseHeaders{
				IdempotencyReplayed: replayedHeader(replayed),
			},
		}
	}),
	http.StatusConflict: internalShape(func(body internalapi.ApiError, _ bool) any {
		return internalapi.SimulationTick409JSONResponse{Body: body}
	}),
	http.StatusServiceUnavailable: internalShape(func(body internalapi.ApiError, _ bool) any {
		return internalapi.SimulationTick503JSONResponse{Body: body}
	}),
}

// internalAnswers is how one internal operation spells the statuses it declares.
type internalAnswers map[int]func(stored []byte, replayed bool) (any, error)

// internalShape reads a stored answer into the contract type one status publishes, which is what makes
// a repeat answer what the first attempt answered rather than a body assembled a second time.
func internalShape[T any](spell func(T, bool) any) func([]byte, bool) (any, error) {
	return func(stored []byte, replayed bool) (any, error) {
		var body T
		if err := json.Unmarshal(stored, &body); err != nil {
			return nil, err
		}
		return spell(body, replayed), nil
	}
}

// spellInternal turns one decided answer into the response object of its operation. A status the
// operation does not declare is a defect of this server rather than a state a client can cause.
func spellInternal(answers internalAnswers, answered rentals.Answered) (any, error) {
	spell, declared := answers[answered.Status]
	if !declared {
		return nil, fmt.Errorf("an internal command answered status %d, which it does not declare",
			answered.Status)
	}
	return spell(answered.Body, answered.Replayed)
}

// internalFailureAnswer spells a failure an internal command could not be decided by. Every failure is
// one of the statuses the operation declares, and it is built here so that both internal operations
// answer a failure the same way.
func internalFailureAnswer(answers internalAnswers, ctx context.Context, err error) (any, error) {
	status, code, message := internalFailure(err)
	reportUncarried(ctx, err, commandFailure{code: servedapi.ErrorCode(code)})
	body, err := json.Marshal(internalapi.ApiError{
		Code:      code,
		Message:   message,
		RequestId: middleware.GetReqID(ctx),
	})
	if err != nil {
		return nil, err
	}
	return spellInternal(answers, rentals.Answered{Status: status, Body: body})
}
