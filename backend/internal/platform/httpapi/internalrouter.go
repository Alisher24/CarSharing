package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/getkin/kin-openapi/openapi3"
)

// SimulationOperations is what the internal surface needs of the fleet model: one tick, advancing
// every simulated vehicle to one moment.
type SimulationOperations interface {
	Tick(ctx context.Context, command rentals.TickCommand) (rentals.Answered, error)
}

// DemoOperations is what the internal surface needs of the demonstration control: one set-to-value
// command, answered once and reproduced on a repeat.
type DemoOperations interface {
	ApplyDemo(ctx context.Context, command rentals.DemoCommand) (rentals.Answered, error)
}

// InternalDependencies is what the internal API is served over: the two capabilities it publishes, the
// credentials each of them is called with, and the profile this installation runs as.
type InternalDependencies struct {
	Simulation SimulationOperations
	Demo       DemoOperations
	Tokens     InternalTokens

	// Demonstrating reports whether this installation is a demonstration. The commands that change
	// the fleet by hand are served only there, which is the decision the process makes about the
	// environment rather than one a handler makes about a request.
	Demonstrating bool
}

// internalHandlers answers the internal operations. It holds the two capabilities and nothing else:
// what a tick or a demonstration command means belongs to the module that decides it.
type internalHandlers struct {
	simulation SimulationOperations
	demo       DemoOperations
}

// NewInternalHandler builds the router of the internal operations.
//
// It reports what it was not given rather than deferring the failure to the first request that reaches
// it, and it fails when a capability has no token: the internal surface is reachable from the internal
// network, so a route without its credential is a route anybody there could call. The external proxy
// answers every path under /internal with an unknown resource, so this surface is not published.
func NewInternalHandler(dependencies InternalDependencies) (http.Handler, error) {
	if err := dependencies.validate(); err != nil {
		return nil, err
	}
	spec, err := servedInternalSpec(dependencies.Demonstrating)
	if err != nil {
		return nil, err
	}
	served := internalHandlers{simulation: dependencies.Simulation, demo: dependencies.Demo}
	strict := internalapi.NewStrictHandlerWithOptions(served, nil, internalStrictErrorHandlers())
	policy := Policy{
		AllowedOrigins: map[string]bool{},
		Authenticate:   dependencies.Tokens.capabilities().authenticate,
	}
	return Boundary(spec, internalapi.Handler(strict), policy), nil
}

func (d InternalDependencies) validate() error {
	for _, required := range []struct {
		name     string
		supplied bool
	}{
		{"fleet model", d.Simulation != nil},
		{"demonstration control", d.Demo != nil},
	} {
		if !required.supplied {
			return fmt.Errorf("%w: %s", ErrIncompleteApplication, required.name)
		}
	}
	return d.Tokens.capabilities().validate()
}

// servedInternalSpec is the internal specification this process serves. The projection carries every
// operation the contract declares, so an operation that is still planned would be routed by being
// generated: it is refused at construction instead, which keeps the declared status and the router
// from drifting apart.
//
// The commands that change the fleet by hand are removed outside a demonstration, so an installation
// that is not one answers them as the unknown resource the contract says an unserved operation is.
func servedInternalSpec(demonstrating bool) (*openapi3.T, error) {
	spec, err := internalapi.GetSwagger()
	if err != nil {
		return nil, err
	}
	for path, pathItem := range spec.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			if operation.Extensions[implementationStatusExtension] != implementedStatus {
				return nil, fmt.Errorf("%s %s is declared but is not implemented by this build", method, path)
			}
		}
	}
	if !demonstrating {
		spec.Paths.Delete(demoActionPath)
	}
	return spec, nil
}

// internalStrictErrorHandlers answers the two failures the generated strict layer reports, which are
// the ones every surface answers: see strictErrorAnswers for what they are.
func internalStrictErrorHandlers() internalapi.StrictHTTPServerOptions {
	failures := strictErrorAnswers()
	return internalapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  failures.request,
		ResponseErrorHandlerFunc: failures.response,
	}
}

// internalRefusal is one refusal as the answer of an internal operation: the status the contract
// declares for it and the encoded error envelope.
type internalRefusal struct {
	status int
	body   []byte
}

// refuse spells a refusal the internal operations answer with. A refusal the internal contract does
// not declare is reported as a defect of this server rather than written under another code.
func refuse(refusal rentals.Refusal) (internalRefusal, error) {
	code, status, message, err := internalRefusalContract(refusal)
	if err != nil {
		return internalRefusal{}, err
	}
	encoded, err := encoded(status, internalapi.ApiError{
		Code:    internalapi.ErrorCode(code),
		Message: message,
	})
	if err != nil {
		return internalRefusal{}, err
	}
	return internalRefusal{status: status, body: encoded.Body}, nil
}

// internalRefusalContract names the code, status and message of one refusal.
func internalRefusalContract(refusal rentals.Refusal) (string, int, string, error) {
	switch refusal.Kind {
	case rentals.VehicleInUse:
		return string(internalapi.VEHICLEINUSE), http.StatusConflict, messageVehicleInUse, nil
	case rentals.SourceNotCarried:
		return string(internalapi.SOURCENOTCARRIED), http.StatusConflict, messageSourceNotCarried, nil
	case rentals.SourceCapacityExceeded:
		return string(internalapi.SOURCECAPACITYEXCEEDED), http.StatusConflict,
			messageSourceCapacityExceeded, nil
	case rentals.VehicleNotFound, rentals.RentalNotFound:
		return string(codeResourceNotFound), http.StatusNotFound, messageResourceNotFound, nil
	default:
		return "", 0, "", fmt.Errorf("the rentals module refused with an unknown kind %q", refusal.Kind)
	}
}

// internalFailure maps a failure of an internal command onto the answer the internal contract declares
// for it. The two idempotency failures are answers a client can repeat; everything else is an outage
// or a defect, which this surface reports the one way a public command does.
func internalFailure(err error) (int, internalapi.ErrorCode, string) {
	switch {
	case errors.Is(err, idempotency.ErrFingerprintMismatch):
		return http.StatusConflict, internalapi.IDEMPOTENCYCONFLICT, messageIdempotencyConflict
	case errors.Is(err, idempotency.ErrInProgress):
		return http.StatusConflict, internalapi.IDEMPOTENCYINPROGRESS, messageIdempotencyInProgress
	default:
		return http.StatusServiceUnavailable, internalapi.SERVICEUNAVAILABLE, messageServiceUnavailable
	}
}

// internalIdentifiers renders a list of identifiers in the shape the contract publishes, which is an
// array and never null.
func internalIdentifiers(identifiers []string) []internalapi.ResourceId {
	rendered := make([]internalapi.ResourceId, 0, len(identifiers))
	for _, identifier := range identifiers {
		rendered = append(rendered, internalapi.ResourceId(identifier))
	}
	return rendered
}

// internalAttemptOf describes one attempt at an internal command: the identifier a repeat is
// recognised by, the fingerprint of what it asked for, and the way its answer is spelled.
func internalAttemptOf(
	identifier, path string, body any, render rentals.Render,
) (rentals.Attempt, error) {
	fingerprint, err := commandFingerprintOf(http.MethodPost, path, body)
	if err != nil {
		return rentals.Attempt{}, err
	}
	return rentals.Attempt{
		Key:         idempotency.Key(identifier),
		Fingerprint: fingerprint,
		Render:      render,
	}, nil
}
