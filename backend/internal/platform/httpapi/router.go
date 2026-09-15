package httpapi

import (
	"net/http"
	"strings"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/getkin/kin-openapi/openapi3"
)

// NewAnonymousRouter serves the operations that need no account: the health probes and the public
// catalog. Everything an account is required for is refused before a handler it was not given is
// reached, which is what a probe outside the application and a routing test need. It holds no
// signals, so the streaming operations are answered as unavailable rather than as unknown resources.
func NewAnonymousRouter(probe ReadinessProbe, catalog Catalog) (http.Handler, error) {
	handlers, err := newCatalogHandlers(catalog)
	if err != nil {
		return nil, err
	}
	served := server{
		health:          health{probe: probe},
		catalogHandlers: handlers,
		streams:         streams{hub: unservedStreams{}},
	}
	policy := transport{allowedOrigins: map[string]bool{}, authenticate: refuseCredentials}
	strict := servedapi.NewStrictHandlerWithOptions(served, nil, strictErrorHandlers())
	return servedRouter(servedapi.Handler(strict), policy), nil
}

// NewHandler builds the router this process serves. It reports the dependencies it was not given
// rather than deferring the failure to the first request that reaches one.
func NewHandler(dependencies Dependencies) (http.Handler, error) {
	accounts, err := newAccounts(dependencies)
	if err != nil {
		return nil, err
	}
	handlers, err := newCatalogHandlers(dependencies.Catalog)
	if err != nil {
		return nil, err
	}
	reservations, err := newReservationHandlers(dependencies.Reservations)
	if err != nil {
		return nil, err
	}
	rides, err := newRideHandlers(dependencies.Reservations)
	if err != nil {
		return nil, err
	}
	finishes, err := newFinishHandlers(dependencies.Reservations)
	if err != nil {
		return nil, err
	}
	payments, err := newPayHandlers(dependencies.Reservations)
	if err != nil {
		return nil, err
	}
	notifications, err := newNotificationHandlers(dependencies.Notifications, dependencies.Cursors)
	if err != nil {
		return nil, err
	}
	invoices, err := newInvoiceHandlers(dependencies.Invoices)
	if err != nil {
		return nil, err
	}
	streaming, err := newStreams(dependencies.Events, dependencies.Sessions)
	if err != nil {
		return nil, err
	}
	served := server{
		health:               health{probe: dependencies.Probe},
		accounts:             accounts,
		catalogHandlers:      handlers,
		reservationHandlers:  reservations,
		rideHandlers:         rides,
		finishHandlers:       finishes,
		payHandlers:          payments,
		notificationHandlers: notifications,
		invoiceHandlers:      invoices,
		streams:              streaming,
	}
	strict := servedapi.NewStrictHandlerWithOptions(served, nil, strictErrorHandlers())
	policy := transport{
		allowedOrigins: originSet(dependencies.AllowedOrigins),
		authenticate:   authenticateSession,
	}
	// The session is attached before the boundary so that the boundary's credential check and the
	// handler below it read one resolved session rather than querying the store twice.
	return withClientAddress(withSession(dependencies.Sessions, servedRouter(servedapi.Handler(strict), policy))), nil
}

// strictErrorHandlers answers the two failures the generated strict layer reports: a request it
// could not decode and a handler that returned a value outside the contract. Both must leave as
// the JSON error envelope rather than as the strict layer's own text.
func strictErrorHandlers() servedapi.StrictHTTPServerOptions {
	return servedapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			writeError(w, r, codeMalformedJSON, messageMalformedJSON)
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			writeError(w, r, codeInternalError, messageInternalError)
		},
	}
}

// NewSurfaceRouter serves the two surfaces of one process from one listener: the internal operations
// under their own prefix, and every other path through the public application. The internal surface is
// not published by the external proxy, which answers every path under its prefix as an unknown
// resource, so reaching it takes a connection to this process rather than to the application.
func NewSurfaceRouter(public http.Handler, internal http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, internalPathPrefix) {
			internal.ServeHTTP(w, r)
			return
		}
		public.ServeHTTP(w, r)
	})
}

// servedRouter wraps one implementation in the transport contract the whole served API shares: the
// specification router and the request-validation boundary. Only the transport policy and the
// implementation differ between the routers this package builds.
func servedRouter(implementation http.Handler, policy transport) http.Handler {
	spec, err := servedapi.GetSwagger()
	if err != nil {
		panic(err)
	}
	dropUnimplementedPaths(spec)
	return boundary(spec, implementation, policy)
}

// dropUnimplementedPaths removes the operations this application does not serve. The generated
// projection also declares the planned operations, but with no operation left on their paths, so
// those paths answer as an unknown resource rather than as a method that exists but is not allowed.
func dropUnimplementedPaths(spec *openapi3.T) {
	for path, pathItem := range spec.Paths.Map() {
		if len(pathItem.Operations()) == 0 {
			spec.Paths.Delete(path)
		}
	}
}
