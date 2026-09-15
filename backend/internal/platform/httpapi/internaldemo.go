package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// demoActionPath is the route of the demonstration control. It is the specification's own path, stated
// once here so that the handler and the fingerprint of the command name the same one.
const demoActionPath = "/internal/v1/demo/actions"

// ApplyDemoAction applies one set-to-value change to the demonstration and answers that it was applied.
func (h internalHandlers) ApplyDemoAction(
	ctx context.Context, request internalapi.ApplyDemoActionRequestObject,
) (internalapi.ApplyDemoActionResponseObject, error) {
	if request.Body == nil {
		return nil, errors.New("a demonstration action must carry its identifier and what it sets")
	}
	command, err := demoCommandOf(*request.Body)
	if err != nil {
		return nil, err
	}
	attempt, err := internalAttemptOf(command.ActionID, demoActionPath, request.Body,
		demoActionRender(command.ActionID))
	if err != nil {
		return nil, err
	}
	command.Attempt = attempt

	answered, err := h.demo.ApplyDemo(ctx, command)
	spelled, err := demoAnswer(ctx, answered, err)
	if err != nil {
		return nil, err
	}
	return spelled.(internalapi.ApplyDemoActionResponseObject), nil
}

// demoCommandOf reads one contract action as the command the rentals module applies. Which fields the
// command carries follows from the discriminator, so a shape that names a source carries no position
// and one that names a ride carries no vehicle.
func demoCommandOf(action internalapi.DemoAction) (rentals.DemoCommand, error) {
	discriminator, err := action.Discriminator()
	if err != nil {
		return rentals.DemoCommand{}, err
	}
	switch discriminator {
	case string(internalapi.SetTelemetryStateActionSetTelemetryState):
		set, err := action.AsSetTelemetryState()
		if err != nil {
			return rentals.DemoCommand{}, err
		}
		return rentals.DemoCommand{
			ActionID:  set.ActionId,
			Kind:      rentals.SetTelemetryState,
			VehicleID: set.VehicleId,
			Online:    set.TelemetryState == internalapi.Online,
		}, nil
	case string(internalapi.SetPositionActionSetPosition):
		set, err := action.AsSetPosition()
		if err != nil {
			return rentals.DemoCommand{}, err
		}
		return rentals.DemoCommand{
			ActionID:  set.ActionId,
			Kind:      rentals.SetPosition,
			VehicleID: set.VehicleId,
			Position: fleet.Position{
				Longitude: float64(set.Position.Coordinates[0]),
				Latitude:  float64(set.Position.Coordinates[1]),
			},
		}, nil
	case string(internalapi.SetEnergyRemainingActionSetEnergyRemaining):
		set, err := action.AsSetEnergyRemaining()
		if err != nil {
			return rentals.DemoCommand{}, err
		}
		remaining, err := fleet.ParseAmount(set.Remaining)
		if err != nil {
			return rentals.DemoCommand{}, fmt.Errorf("the reserve of a demonstration refill: %w", err)
		}
		return rentals.DemoCommand{
			ActionID:  set.ActionId,
			Kind:      rentals.SetEnergyRemaining,
			VehicleID: set.VehicleId,
			Source:    fleet.SourceKind(set.SourceKind),
			Remaining: remaining,
		}, nil
	case string(internalapi.MarkServicedActionMarkServiced):
		set, err := action.AsMarkServiced()
		if err != nil {
			return rentals.DemoCommand{}, err
		}
		return rentals.DemoCommand{
			ActionID:  set.ActionId,
			Kind:      rentals.MarkServiced,
			VehicleID: set.VehicleId,
		}, nil
	case string(internalapi.SetNextPaymentOutcomeActionSetNextPaymentOutcome):
		set, err := action.AsSetNextPaymentOutcome()
		if err != nil {
			return rentals.DemoCommand{}, err
		}
		return rentals.DemoCommand{
			ActionID: set.ActionId,
			Kind:     rentals.SetNextPaymentOutcome,
			RentalID: set.RentalId,
			Outcome:  invoices.DemoOutcome(set.Outcome),
		}, nil
	default:
		return rentals.DemoCommand{}, fmt.Errorf(
			"the demonstration action %q is not one this build applies", discriminator)
	}
}

// demoActionRender spells what a demonstration command decided: that it was applied, or the refusal it
// decided on instead.
func demoActionRender(actionID string) rentals.Render {
	return func(outcome rentals.Outcome) (rentals.Response, error) {
		if outcome.Refused() {
			refused, err := refuse(outcome.Refusal)
			if err != nil {
				return rentals.Response{}, err
			}
			return rentals.Response{Status: refused.status, Body: refused.body}, nil
		}
		return encoded(http.StatusOK, internalapi.DemoResult{
			ActionId:   actionID,
			ServerTime: timestamp.Format(outcome.Moment),
		})
	}
}

// demoAnswer spells what a demonstration command decided, or the failure that kept it from being
// decided.
func demoAnswer(ctx context.Context, answered rentals.Answered, failure error) (any, error) {
	if failure != nil {
		return internalFailureAnswer(demoActionAnswers, ctx, failure)
	}
	return spellInternal(demoActionAnswers, answered)
}

// demoActionAnswers is every status the demonstration operation declares, each spelled as the
// operation's own response type.
var demoActionAnswers = internalAnswers{
	http.StatusOK: internalShape(func(body internalapi.DemoResult, replayed bool) any {
		return internalapi.ApplyDemoAction200JSONResponse{
			Body: body,
			Headers: internalapi.ApplyDemoAction200ResponseHeaders{
				IdempotencyReplayed: replayedHeader(replayed),
			},
		}
	}),
	http.StatusConflict: internalShape(func(body internalapi.ApiError, replayed bool) any {
		return internalapi.ApplyDemoAction409JSONResponse{
			Body: body,
			Headers: internalapi.ApplyDemoAction409ResponseHeaders{
				IdempotencyReplayed: replayedHeader(replayed),
				RetryAfter:          retryAfterOfInternal(body),
			},
		}
	}),
	http.StatusNotFound: internalShape(func(body internalapi.ApiError, _ bool) any {
		return internalapi.ApplyDemoAction404JSONResponse{Body: body}
	}),
	http.StatusServiceUnavailable: internalShape(func(body internalapi.ApiError, _ bool) any {
		return internalapi.ApplyDemoAction503JSONResponse{Body: body}
	}),
}

// retryAfterOfInternal asks for a wait when a demonstration command met another attempt at the same
// key. Every other refusal of this surface is a decision rather than a race, and carries no wait.
func retryAfterOfInternal(body internalapi.ApiError) *int {
	if body.Code != internalapi.IDEMPOTENCYINPROGRESS {
		return nil
	}
	return retryAfterCommandBusy()
}
