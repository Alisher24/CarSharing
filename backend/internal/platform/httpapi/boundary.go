package httpapi

import (
	"net/http"
	"time"

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

// Policy is what one listener applies to every request beyond the specification itself: the browser
// origins a mutation may come from, and the credentials an operation that declares one is checked
// against. The owning application supplies it; the isolated contract routers supply their own so that
// they exercise the same steps.
type Policy struct {
	// AllowedOrigins is the set of origins a browser mutation may come from. An empty set refuses
	// every origin, which is what a listener that serves no browser at all declares.
	AllowedOrigins map[string]bool

	// Authenticate checks the credential of an operation that declares a security requirement. A
	// listener whose operations declare none never calls it.
	Authenticate openapi3filter.AuthenticationFunc
}

// Boundary applies the same transport contract to every surface of this program: request identity,
// panic recovery, origin and CSRF checks, authentication, body limits, media type and schema
// validation. Each listener serves its own specification over its own implementation.
func Boundary(spec *openapi3.T, next http.Handler, policy Policy) http.Handler {
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
		releaseWriteDeadline(w, route.Operation)
		validate.ServeHTTP(w, pending.request)
	})
}

// releaseWriteDeadline removes the server's write deadline for an operation whose response is a
// stream. That deadline bounds an ordinary request, and a connection that stays open for as long as
// the browser holds it would be cut off by it; the stream bounds its own writes instead, with the
// limit the contract states. A response writer that cannot carry a deadline — a recorder in a test —
// is left as it is.
func releaseWriteDeadline(w http.ResponseWriter, operation *openapi3.Operation) {
	if !declaresStreamingResponse(operation) {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
}

// declaresStreamingResponse reports whether any response of an operation is a stream, which the
// contract states through the media type it declares.
func declaresStreamingResponse(operation *openapi3.Operation) bool {
	for _, response := range operation.Responses.Map() {
		if response.Value.Content.Get(streamMediaType) != nil {
			return true
		}
	}
	return false
}

// transportSteps is the policy every request passes through, in the order it must be applied. A
// request from a foreign origin is refused before anything else looks at it, so it can create no
// account, session or cookie on the way past. Credentials are then checked before any step touches
// the body, so an unauthenticated caller cannot learn whether its payload would have parsed, and
// the body is buffered before the schema validator, which reads it a second time.
func transportSteps(spec *openapi3.T, policy Policy) []boundaryStep {
	return []boundaryStep{
		requireAllowedOrigin(policy.AllowedOrigins),
		requireSessionCSRFToken,
		requireCredentials(spec, policy.Authenticate),
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
