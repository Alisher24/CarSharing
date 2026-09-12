package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Alisher24/CarSharing/backend/internal/contracts/formats"
	healthapi "github.com/Alisher24/CarSharing/backend/internal/contracts/healthapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

const (
	// defaultBodyLimitBytes is the largest request body an operation accepts unless it declares
	// x-body-limit. The internal contracts raise it because a simulation tick carries a batch.
	defaultBodyLimitBytes = 64 << 10

	jsonMediaType = "application/json"
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
type boundaryStep func(*boundaryRequest) *apiError

// boundary applies the same transport contract to the production router and to the isolated
// contract routers: request identity, panic recovery, authentication, body limits, media type and
// schema validation. Authentication is supplied by the owning application; the health projection
// has no security requirements.
func boundary(spec *openapi3.T, next http.Handler, authenticate ...openapi3filter.AuthenticationFunc) http.Handler {
	routes := mustResolveRoutes(spec)
	validate := schemaValidated(spec, next)
	// The order is load-bearing. Credentials are checked before any step touches the body, so an
	// unauthenticated caller cannot learn whether its payload would have parsed, and the body is
	// buffered before the schema validator, which reads it a second time.
	steps := []boundaryStep{
		requireCredentials(spec, singleAuthenticationFunc(authenticate)),
		bufferBodyWithinLimit,
		requireJSONRequestBody,
		requireSingleValuedHeaders,
	}
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

// mustResolveRoutes builds the router for a specification. A specification that cannot be routed is
// a defect in a generated contract rather than a runtime condition, so it fails at construction.
func mustResolveRoutes(spec *openapi3.T) routers.Router {
	routes, err := gorillamux.NewRouter(spec)
	if err != nil {
		panic(err)
	}
	return routes
}

func singleAuthenticationFunc(authenticate []openapi3filter.AuthenticationFunc) openapi3filter.AuthenticationFunc {
	if len(authenticate) > 0 {
		return authenticate[0]
	}
	return nil
}

// withRequestIdentity assigns the request an identifier and echoes it on the response together with
// the no-store policy every API response carries. It runs before routing so that an error written
// for an unroutable path still reports an identifier a client can quote.
func withRequestIdentity(w http.ResponseWriter, r *http.Request) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), middleware.RequestIDKey, uuid.NewString()))
	w.Header().Set("X-Request-ID", requestID(r))
	w.Header().Set("Cache-Control", "no-store")
	return r
}

func requestID(r *http.Request) string {
	return middleware.GetReqID(r.Context())
}

// writePanicAsInternalError answers a panic below the boundary with the error contract, so that a
// defect reaches the client as a complete JSON error instead of a truncated response.
func writePanicAsInternalError(w http.ResponseWriter, r *http.Request) {
	if recovered := recover(); recovered != nil {
		writeError(w, r, codeInternalError, messageInternalError)
	}
}

// schemaValidated runs the generated request validator and, once it accepts the request, the
// positional constraints collected earlier. Security requirements are already satisfied by
// requireCredentials, so the validator is told not to check them again.
func schemaValidated(spec *openapi3.T, next http.Handler) http.Handler {
	options := &nethttpmiddleware.Options{
		Options: openapi3filter.Options{MultiError: true, AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		ErrorHandlerWithOpts: func(
			_ context.Context, err error,
			w http.ResponseWriter, r *http.Request, _ nethttpmiddleware.ErrorHandlerOpts,
		) {
			writeRequestError(spec, w, r, err)
		},
	}
	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, options)(constraintGate(next))
}

// constraintGate rejects the positional constraints requireJSONRequestBody recorded on the request.
// They are computed before validation because they need the parsed payload, but they must not
// pre-empt the schema: a schema failure reports them alongside its own violations, and this gate
// reports them only when the schema itself is satisfied.
func constraintGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if violations, ok := r.Context().Value(constraintsKey{}).([]violation); ok && len(violations) > 0 {
			writeError(w, r, codeValidationFailed, messageValidationFailed, violations...)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireCredentials rejects a request that does not satisfy the security requirements of its
// operation, or of the specification when the operation declares none.
func requireCredentials(spec *openapi3.T, authenticate openapi3filter.AuthenticationFunc) boundaryStep {
	return func(b *boundaryRequest) *apiError {
		requirements := b.route.Operation.Security
		if requirements == nil {
			requirements = &spec.Security
		}
		if len(*requirements) == 0 {
			return nil
		}
		// Credential validation cannot consume or parse the domain payload.
		withoutBody := b.request.Clone(b.request.Context())
		withoutBody.Body = http.NoBody
		input := &openapi3filter.RequestValidationInput{
			Request:    withoutBody,
			PathParams: b.pathParams,
			Route:      b.route,
			Options:    &openapi3filter.Options{AuthenticationFunc: authenticate},
		}
		if err := openapi3filter.ValidateSecurityRequirements(b.request.Context(), input, *requirements); err != nil {
			return &apiError{code: authenticationCode(b.request.URL.Path), message: "Authentication required"}
		}
		return nil
	}
}

// authenticationCode keeps the internal API's authentication failure distinguishable from a public
// one, so a caller of /internal cannot read it as an expired session.
func authenticationCode(path string) healthapi.ErrorCode {
	if strings.HasPrefix(path, "/internal/") {
		return codeInternalAuthenticationRequired
	}
	return codeAuthenticationRequired
}

// bufferBodyWithinLimit reads the body once within the limit of its operation and replaces it with a
// replayable reader, because the schema validator and the handler each read it again.
func bufferBodyWithinLimit(b *boundaryRequest) *apiError {
	if b.request.Body == nil {
		b.request.Body = http.NoBody
	}
	limited := http.MaxBytesReader(b.writer, b.request.Body, bodyLimit(b.route.Operation))
	body, err := io.ReadAll(limited)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &apiError{code: codeBodyTooLarge, message: "Request body is too large"}
		}
		return &apiError{code: codeMalformedJSON, message: "Request body could not be read"}
	}
	b.body = body
	b.request.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

func bodyLimit(operation *openapi3.Operation) int64 {
	if declared, ok := operation.Extensions["x-body-limit"].(float64); ok {
		return int64(declared)
	}
	return defaultBodyLimitBytes
}

// requireJSONRequestBody enforces the presence, media type and syntax the operation declares, then
// records the positional constraints OpenAPI 3.0 cannot express so that constraintGate can report
// them once the schema validator has had its say.
func requireJSONRequestBody(b *boundaryRequest) *apiError {
	if b.route.Operation.RequestBody == nil {
		if len(b.body) > 0 {
			return validationFailure(bodyViolation("", "unexpected_body", "Request body is not allowed"))
		}
		return nil
	}
	if len(b.body) == 0 {
		return validationFailure(bodyViolation("", "required", "Request body is required"))
	}
	media, _, err := mime.ParseMediaType(b.request.Header.Get("Content-Type"))
	if err != nil || media != jsonMediaType {
		return &apiError{code: codeUnsupportedMediaType, message: "Expected " + jsonMediaType}
	}
	if !json.Valid(b.body) {
		return &apiError{code: codeMalformedJSON, message: messageMalformedJSON}
	}
	var payload any
	_ = json.Unmarshal(b.body, &payload)
	schema := b.route.Operation.RequestBody.Value.Content[jsonMediaType].Schema.Value
	var violations []violation
	for _, constraint := range formats.Constraints(schema, payload) {
		violations = append(violations, bodyViolation(constraint.Pointer, "invalid", constraint.Message))
	}
	b.request = b.request.WithContext(context.WithValue(b.request.Context(), constraintsKey{}, violations))
	return nil
}

// requireSingleValuedHeaders rejects a repeated declared header, which would otherwise let a caller
// smuggle a second value past a validator that reads only the first.
func requireSingleValuedHeaders(b *boundaryRequest) *apiError {
	for _, parameter := range b.route.Operation.Parameters {
		if parameter.Value.In != "header" {
			continue
		}
		if len(b.request.Header.Values(parameter.Value.Name)) > 1 {
			return &apiError{code: codeInvalidHeader, message: "Header must occur only once"}
		}
	}
	return nil
}

func validationFailure(violations ...violation) *apiError {
	return &apiError{code: codeValidationFailed, message: messageValidationFailed, violations: violations}
}
