package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/democontrol"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
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
	attempt, err := commandAttempt(idempotency.Key(command.ActionID), demoActionPath, request.Body,
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
// command carries follows from the action's own declaration: a shape that names a source carries no
// position and one that names a ride carries no vehicle. The declaration is the demonstration
// vocabulary's, so a new action is a row there rather than a case here.
func demoCommandOf(action internalapi.DemoAction) (rentals.DemoCommand, error) {
	body, err := json.Marshal(action)
	if err != nil {
		return rentals.DemoCommand{}, err
	}
	stated, err := democontrol.Read(body)
	if err != nil {
		return rentals.DemoCommand{}, err
	}
	return demoCommand(stated)
}

// demoCommand builds the command the module applies out of what one request stated. Every field is
// read by the row that knows it, so an action stating fields this surface already reads is a row of the
// vocabulary rather than another branch.
func demoCommand(stated democontrol.Stated) (rentals.DemoCommand, error) {
	command := rentals.DemoCommand{
		ActionID: stated.ActionID,
		Kind:     rentals.DemoActionKind(stated.Action),
	}
	for field, values := range stated.Values {
		read, known := demoFields[field]
		if !known {
			return rentals.DemoCommand{}, fmt.Errorf(
				"the demonstration field %q is not one this build reads", field)
		}
		if err := read(&command, values); err != nil {
			return rentals.DemoCommand{}, err
		}
	}
	return command, nil
}

// demoFields is what each field of a demonstration request states about the command the module applies:
// one row per field of the vocabulary rather than per action.
var demoFields = map[democontrol.Field]func(*rentals.DemoCommand, []string) error{
	democontrol.VehicleIDField: func(command *rentals.DemoCommand, stated []string) error {
		command.VehicleID = stated[0]
		return nil
	},
	democontrol.TelemetryStateField: func(command *rentals.DemoCommand, stated []string) error {
		command.Online = stated[0] == string(internalapi.Online)
		return nil
	},
	democontrol.PositionField: func(command *rentals.DemoCommand, stated []string) error {
		longitude, err := strconv.ParseFloat(stated[0], 64)
		if err != nil {
			return fmt.Errorf("the longitude of a demonstration position: %w", err)
		}
		latitude, err := strconv.ParseFloat(stated[1], 64)
		if err != nil {
			return fmt.Errorf("the latitude of a demonstration position: %w", err)
		}
		command.Position = fleet.Position{Longitude: longitude, Latitude: latitude}
		return nil
	},
	democontrol.SourceKindField: func(command *rentals.DemoCommand, stated []string) error {
		command.Source = fleet.SourceKind(stated[0])
		return nil
	},
	democontrol.RemainingField: func(command *rentals.DemoCommand, stated []string) error {
		remaining, err := fleet.ParseAmount(stated[0])
		if err != nil {
			return fmt.Errorf("the reserve of a demonstration refill: %w", err)
		}
		command.Remaining = remaining
		return nil
	},
	democontrol.RentalIDField: func(command *rentals.DemoCommand, stated []string) error {
		command.RentalID = stated[0]
		return nil
	},
	democontrol.OutcomeField: func(command *rentals.DemoCommand, stated []string) error {
		command.Outcome = invoices.DemoOutcome(stated[0])
		return nil
	},
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
		reportUncarried(ctx, failure, commandFailureOf(failure))
		return spellFailure(demoActionOperation, ctx, commandFailureOf(failure))
	}
	return answerOf(demoActionOperation, answered)
}

// demoActionOperation declares every status the demonstration operation declares, each spelled as the
// operation's own response type.
var demoActionOperation = commandOperation{
	name:    "demonstration action",
	path:    demoActionPath,
	refused: http.StatusConflict,
	answers: map[int]answerShape{
		http.StatusOK: shapeOf(func(body internalapi.DemoResult, headers answerHeaders) any {
			return internalapi.ApplyDemoAction200JSONResponse{
				Body: body,
				Headers: internalapi.ApplyDemoAction200ResponseHeaders{
					IdempotencyReplayed: replayedHeader(headers.replayed),
				},
			}
		}),
		http.StatusConflict: shapeOf(func(body internalapi.ApiError, headers answerHeaders) any {
			return internalapi.ApplyDemoAction409JSONResponse{
				Body: body,
				Headers: internalapi.ApplyDemoAction409ResponseHeaders{
					IdempotencyReplayed: replayedHeader(headers.replayed),
					RetryAfter:          headers.retry(servedapi.ErrorCode(body.Code)),
				},
			}
		}),
		http.StatusNotFound: shapeOf(func(body internalapi.ApiError, headers answerHeaders) any {
			return internalapi.ApplyDemoAction404JSONResponse{Body: body}
		}),
		http.StatusServiceUnavailable: shapeOf(func(body internalapi.ApiError, headers answerHeaders) any {
			return internalapi.ApplyDemoAction503JSONResponse{Body: body}
		}),
	},
}
