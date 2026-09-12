package httpapi

import "github.com/getkin/kin-openapi/openapi3"

// The header names whose behaviour this package defines, the media type it accepts, and the cache
// policy it answers every response with.
const (
	originHeader            = "Origin"
	csrfTokenHeader         = "X-CSRF-Token"
	idempotencyKeyHeader    = "Idempotency-Key"
	deliveryKeyHeader       = "Delivery-Key"
	contentTypeHeader       = "Content-Type"
	requestIDHeader         = "X-Request-ID"
	cacheControlHeader      = "Cache-Control"
	noStoreCacheControl     = "no-store"
	jsonMediaType           = "application/json"
	headerParameterLocation = "header"
)

// declaresHeader reports whether an operation declares a header parameter, optionally demanding
// that the contract marks it required.
func declaresHeader(operation *openapi3.Operation, name string, required bool) bool {
	for _, parameter := range operation.Parameters {
		if parameter.Value.In == headerParameterLocation && parameter.Value.Name == name {
			return !required || parameter.Value.Required
		}
	}
	return false
}

// requireSingleValuedHeaders rejects a repeated declared header, which would otherwise let a caller
// smuggle a second value past a validator that reads only the first.
func requireSingleValuedHeaders(b *boundaryRequest) *apiError {
	for _, parameter := range b.route.Operation.Parameters {
		if parameter.Value.In != headerParameterLocation {
			continue
		}
		if len(b.request.Header.Values(parameter.Value.Name)) > 1 {
			return &apiError{code: codeInvalidHeader, message: messageInvalidHeader}
		}
	}
	return nil
}
