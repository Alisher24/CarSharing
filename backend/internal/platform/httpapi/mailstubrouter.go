package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	mailstubapi "github.com/Alisher24/CarSharing/backend/internal/contracts/mailstubapi"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5/middleware"
)

// The two listeners the mail stub serves. Which one serves a path is the contract's own x-listener
// extension, so a path joins a listener by being written down in the specification rather than by
// being added to a second list beside it.
const (
	internalMailListener = "internal"
	inboxMailListener    = "inbox"

	// listenerExtension is the contract's word for the listener a path belongs to.
	listenerExtension = "x-listener"
)

// mailDemoActionPath is the route of the mail stub's demonstration control. It is this contract's own
// path, stated once here so that the projection that removes it outside a demonstration and the
// fingerprint of the command name the same one.
const mailDemoActionPath = "/internal/v1/demo/actions"

// errUnservedMailOperation reports an operation a listener was asked for although its specification
// does not declare it. The projections remove the operations of the other listener, so a request
// cannot reach one; a projection that stopped removing them would be reported here rather than
// answered by the wrong surface.
var errUnservedMailOperation = errors.New("this listener does not serve that operation")

// MailDeliveries is what the delivery operation needs of the box: one letter accepted under its key,
// answered with the letter the key holds and whether the answer is the one a demonstration asked to
// lose.
type MailDeliveries interface {
	Accept(ctx context.Context, key mailstub.Key, request mailstub.Request) (mailstub.Receipt, error)
}

// MailActions is what the demonstration operation needs of the remembered actions: one demand armed
// under an identifier, or the answer that identifier already holds.
type MailActions interface {
	Apply(
		ctx context.Context, actionID string, fingerprint idempotency.Fingerprint,
	) (mailstub.Result, bool, error)
}

// MailInbox is what the two read operations need of the box: one letter, and one page of them.
type MailInbox interface {
	ByID(ctx context.Context, id string) (mailstub.Message, error)
	ReadPage(ctx context.Context, after *mailstub.Position, limit int) (mailstub.Page, error)
}

// MailstubInternalDependencies is what the internal listener of the mail stub is served over: the two
// operations it publishes, the probe its container checks, the credentials they are called with, and
// the profile this installation runs as.
type MailstubInternalDependencies struct {
	Deliveries MailDeliveries
	Actions    MailActions
	Probe      MailReadinessProbe
	Tokens     MailstubTokens

	// Demonstrating reports whether this installation is a demonstration. The action that arms the
	// loss of an answer is served only there, which is the decision the process makes about the
	// environment rather than one a handler makes about a request.
	Demonstrating bool
}

// MailstubInboxDependencies is what the read-only inbox is served over: the box it reads and the
// signer its cursors are issued under.
type MailstubInboxDependencies struct {
	Inbox   MailInbox
	Cursors *cursor.Signer
}

// NewMailstubInternalListener builds the internal listener of the mail stub: the delivery operation,
// the demonstration control, and the readiness probe of the container they run in.
//
// It reports what it was not given rather than deferring the failure to the first request, and it
// fails when a capability has no token: this listener is reachable from the internal network, so a
// route without its credential is a route anybody there could call. The external proxy answers every
// path under /internal with an unknown resource, so this surface is not published.
func NewMailstubInternalListener(dependencies MailstubInternalDependencies) (http.Handler, error) {
	if err := dependencies.validate(); err != nil {
		return nil, err
	}
	spec, err := mailstubSpec(internalMailListener, dependencies.Demonstrating)
	if err != nil {
		return nil, err
	}
	served := mailstubInternalHandlers{deliveries: dependencies.Deliveries, actions: dependencies.Actions}
	strict := mailstubapi.NewStrictHandlerWithOptions(served, nil, mailstubStrictErrorHandlers())
	policy := Policy{
		AllowedOrigins: map[string]bool{},
		Authenticate:   dependencies.Tokens.capabilities().authenticate,
	}
	return readinessBeside(Boundary(spec, mailstubapi.Handler(strict), policy), dependencies.Probe), nil
}

func (d MailstubInternalDependencies) validate() error {
	for _, required := range []struct {
		name     string
		supplied bool
	}{
		{"mail box", d.Deliveries != nil},
		{"demonstration control", d.Actions != nil},
		{"readiness probe", d.Probe != nil},
	} {
		if !required.supplied {
			return fmt.Errorf("%w: %s", ErrIncompleteApplication, required.name)
		}
	}
	return d.Tokens.capabilities().validate()
}

// NewMailstubInboxListener builds the listener of the read-only inbox: two anonymous reads of the
// box, published on the loopback address of the container rather than through the proxy, and the
// human pages of the same box beside them.
func NewMailstubInboxListener(dependencies MailstubInboxDependencies) (http.Handler, error) {
	if dependencies.Inbox == nil {
		return nil, fmt.Errorf("%w: mail box", ErrIncompleteApplication)
	}
	if dependencies.Cursors == nil {
		return nil, fmt.Errorf("%w: cursor signer", ErrIncompleteApplication)
	}
	spec, err := mailstubSpec(inboxMailListener, false)
	if err != nil {
		return nil, err
	}
	served := mailstubInboxHandlers{inbox: dependencies.Inbox, cursors: dependencies.Cursors}
	strict := mailstubapi.NewStrictHandlerWithOptions(served, nil, mailstubStrictErrorHandlers())
	policy := Policy{AllowedOrigins: map[string]bool{}, Authenticate: refuseCredentials}
	operations := Boundary(spec, mailstubapi.Handler(strict), policy)
	return inboxPagesBeside(operations, dependencies.Inbox, dependencies.Cursors,
		newContractPrefixes(spec)), nil
}

// readinessBeside serves the container's readiness probe beside the contract. The probe is the
// container's own check rather than an operation of the mail stub's contract, so it is answered
// before the boundary, which knows only the paths the contract declares; a request that is not the
// probe's own method is left to the boundary, which answers it as the unknown resource it is.
func readinessBeside(operations http.Handler, probe MailReadinessProbe) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ReadyPath || r.Method != http.MethodGet {
			operations.ServeHTTP(w, r)
			return
		}
		r = withRequestIdentity(w, r)
		ctx, cancel := context.WithTimeout(r.Context(), readinessProbeTimeout)
		defer cancel()
		if err := probe(ctx); err != nil {
			writeError(w, r, codeServiceUnavailable, messageServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

// mailstubSpec is the specification one listener serves. The projection carries every operation the
// contract declares, so an operation that is still planned would be routed by being generated: it is
// refused at construction instead, which keeps the declared status and the router from drifting
// apart.
//
// The operations of the other listener are removed, so a listener serves exactly the paths the
// contract marks as its own. The demonstration control is removed outside a demonstration, so an
// installation that is not one answers it as the unknown resource the contract says an unserved
// operation is.
func mailstubSpec(listener string, demonstrating bool) (*openapi3.T, error) {
	spec, err := mailstubapi.GetSwagger()
	if err != nil {
		return nil, err
	}
	for path, pathItem := range spec.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			if operation.Extensions[implementationStatusExtension] != implementedStatus {
				return nil, fmt.Errorf("%s %s is declared but is not implemented by this build", method, path)
			}
			if operation.Extensions[listenerExtension] != listener {
				pathItem.SetOperation(method, nil)
			}
		}
		if len(pathItem.Operations()) == 0 {
			spec.Paths.Delete(path)
		}
	}
	if !demonstrating {
		spec.Paths.Delete(mailDemoActionPath)
	}
	return spec, nil
}

// mailstubStrictErrorHandlers answers the two failures the generated strict layer reports, which are
// the ones every surface answers: see strictErrorAnswers for what they are.
func mailstubStrictErrorHandlers() mailstubapi.StrictHTTPServerOptions {
	failures := strictErrorAnswers()
	return mailstubapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  failures.request,
		ResponseErrorHandlerFunc: failures.response,
	}
}

// mailstubError renders the error envelope for one mail stub answer. All four specifications declare
// the same envelope, so the body is built here and the code names this contract's own constant.
func mailstubError(ctx context.Context, code mailstubapi.ErrorCode, message string) mailstubapi.ApiError {
	return mailstubapi.ApiError{Code: code, Message: message, RequestId: middleware.GetReqID(ctx)}
}

// mailstubUnserved answers the operations a listener does not serve. Which ones those are is decided
// by the contract projection above rather than here, so these implementations exist only to report a
// projection that stopped removing them.
type mailstubUnserved struct{}

func (mailstubUnserved) GetMessages(
	context.Context, mailstubapi.GetMessagesRequestObject,
) (mailstubapi.GetMessagesResponseObject, error) {
	return nil, errUnservedMailOperation
}

func (mailstubUnserved) GetMessage(
	context.Context, mailstubapi.GetMessageRequestObject,
) (mailstubapi.GetMessageResponseObject, error) {
	return nil, errUnservedMailOperation
}

func (mailstubUnserved) DeliverMessage(
	context.Context, mailstubapi.DeliverMessageRequestObject,
) (mailstubapi.DeliverMessageResponseObject, error) {
	return nil, errUnservedMailOperation
}

func (mailstubUnserved) ApplyMailDemoAction(
	context.Context, mailstubapi.ApplyMailDemoActionRequestObject,
) (mailstubapi.ApplyMailDemoActionResponseObject, error) {
	return nil, errUnservedMailOperation
}
