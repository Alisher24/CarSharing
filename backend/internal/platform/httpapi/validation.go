package httpapi

import (
	"context"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

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
