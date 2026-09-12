package httpapi

import (
	"net/http"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/getkin/kin-openapi/openapi3"
)

// NewProbeRouter serves the health operations over one readiness probe and nothing else, which is
// what a probe outside the application and a routing test need. It refuses the operations that
// depend on the account rules before reaching for a handler they were not given.
func NewProbeRouter(probe ReadinessProbe) http.Handler {
	served := server{health: health{probe: probe}}
	policy := transport{allowedOrigins: map[string]bool{}, authenticate: refuseCredentials}
	return servedRouter(servedapi.Handler(servedapi.NewStrictHandler(served, nil)), policy)
}

// NewHandler builds the router this process serves. It reports the dependencies it was not given
// rather than deferring the failure to the first request that reaches one.
func NewHandler(dependencies Dependencies) (http.Handler, error) {
	accounts, err := newAccounts(dependencies)
	if err != nil {
		return nil, err
	}
	served := server{health: health{probe: dependencies.Probe}, accounts: accounts}
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
