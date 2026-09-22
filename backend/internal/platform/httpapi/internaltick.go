package httpapi

import (
	"context"
	"errors"
	"net/http"

	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
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
	attempt, err := commandAttempt(idempotency.Key(request.Body.TickId), simulationTickPath, request.Body,
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
		reportUncarried(ctx, failure, commandFailureOf(failure))
		return spellFailure(tickOperation, ctx, commandFailureOf(failure))
	}
	return answerOf(tickOperation, answered)
}

// tickOperation declares every status the tick operation declares, each spelled as the operation's own
// response type, so an answer stored under a tick's key is published as the tick that was asked.
var tickOperation = commandOperation{
	name:    "simulation tick",
	path:    simulationTickPath,
	refused: http.StatusConflict,
	answers: map[int]answerShape{
		http.StatusOK: shapeOf(func(body internalapi.TickResult, headers answerHeaders) any {
			return internalapi.SimulationTick200JSONResponse{
				Body: body,
				Headers: internalapi.SimulationTick200ResponseHeaders{
					IdempotencyReplayed: replayedHeader(headers.replayed),
				},
			}
		}),
		http.StatusConflict: shapeOf(func(body internalapi.ApiError, headers answerHeaders) any {
			return internalapi.SimulationTick409JSONResponse{Body: body}
		}),
		http.StatusServiceUnavailable: shapeOf(func(body internalapi.ApiError, headers answerHeaders) any {
			return internalapi.SimulationTick503JSONResponse{Body: body}
		}),
	},
}
