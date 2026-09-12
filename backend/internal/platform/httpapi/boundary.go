package httpapi

import (
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// boundaryRequest is one in-flight request together with the route the specification matched and
// the body buffered for it, which the steps after the read all inspect.
type boundaryRequest struct {
	writer     http.ResponseWriter
	request    *http.Request
	route      *routers.Route
	pathParams map[string]string
	body       []byte
}

// boundaryStep reports the first transport failure it finds in a request, or nil to continue. A
// step may replace the request to buffer the body or to carry its results to a later step.
type boundaryStep func(*boundaryRequest) *contractError

// transport is the policy the boundary applies to every request: which browser origins may make a
// mutation, and how a session credential is checked. The owning application supplies it; the
// isolated contract routers supply their own so that they exercise the same steps.
type transport struct {
	allowedOrigins map[string]bool
	authenticate   openapi3filter.AuthenticationFunc
}

// boundary applies the same transport contract to the production router and to the isolated
// contract routers: request identity, panic recovery, origin and CSRF checks, authentication, body
// limits, media type and schema validation.
func boundary(spec *openapi3.T, next http.Handler, policy transport) http.Handler {
	routes := mustResolveRoutes(spec)
	validate := schemaValidated(spec, next)
	steps := transportSteps(spec, policy)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = withRequestIdentity(w, r)
		defer writePanicAsInternalError(w, r)

		route, pathParams, err := routes.FindRoute(r)
		if err != nil {
			writeRequestError(spec, w, r, err)
			return
		}

		pending := &boundaryRequest{writer: w, request: r, route: route, pathParams: pathParams}
		for _, step := range steps {
			if failure := step(pending); failure != nil {
				writeError(w, pending.request, failure.code, failure.message, failure.violations...)
				return
			}
		}
		validate.ServeHTTP(w, pending.request)
	})
}

// transportSteps is the policy every request passes through, in the order it must be applied. A
// request from a foreign origin is refused before anything else looks at it, so it can create no
// account, session or cookie on the way past. Credentials are then checked before any step touches
// the body, so an unauthenticated caller cannot learn whether its payload would have parsed, and
// the body is buffered before the schema validator, which reads it a second time.
func transportSteps(spec *openapi3.T, policy transport) []boundaryStep {
	return []boundaryStep{
		requireAllowedOrigin(policy.allowedOrigins),
		requireSessionCSRFToken,
		requireCredentials(spec, policy.authenticate),
		bufferBodyWithinLimit,
		requireJSONRequestBody,
		requireSingleValuedHeaders,
	}
}

// mustResolveRoutes builds the router for a specification. A specification that cannot be routed is
// a defect in a generated contract rather than a runtime condition, so it fails at construction.
func mustResolveRoutes(spec *openapi3.T) routers.Router {
	routes, err := gorillamux.NewRouter(spec)
	if err != nil {
		panic(err)
	}
	return routes
}
