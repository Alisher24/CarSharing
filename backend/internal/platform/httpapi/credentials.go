package httpapi

import (
	"errors"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
)

// requireCredentials rejects a request that does not satisfy the security requirements of its
// operation, or of the specification when the operation declares none.
func requireCredentials(spec *openapi3.T, authenticate openapi3filter.AuthenticationFunc) boundaryStep {
	return func(b *boundaryRequest) *contractError {
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
			if errors.Is(err, errSessionStoreUnavailable) {
				return &contractError{code: codeServiceUnavailable, message: messageServiceUnavailable}
			}
			return &contractError{code: authenticationCode(b.request.URL.Path), message: messageAuthenticationRequired}
		}
		return nil
	}
}
