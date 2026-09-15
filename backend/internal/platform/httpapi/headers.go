package httpapi

import "github.com/getkin/kin-openapi/openapi3"

// The header names whose behaviour this package defines, the media types it accepts and answers with,
// and the cache policy every response carries.
const (
	originHeader         = "Origin"
	csrfTokenHeader      = "X-CSRF-Token"
	idempotencyKeyHeader = "Idempotency-Key"
	deliveryKeyHeader    = "Delivery-Key"
	authorizationHeader  = "Authorization"
	contentTypeHeader    = "Content-Type"
	requestIDHeader      = "X-Request-ID"
	cacheControlHeader   = "Cache-Control"
	noStoreCacheControl  = "no-store"
	jsonMediaType        = "application/json"
	streamMediaType      = "text/event-stream"
)

// The extension the contract states the implementation status of an operation in, and the one status
// that means this build serves it.
const (
	implementationStatusExtension = "x-implementation-status"
	implementedStatus             = "implemented"
)

// declaresHeader reports whether an operation declares a header parameter, optionally demanding
// that the contract marks it required.
func declaresHeader(operation *openapi3.Operation, name string, required bool) bool {
	for _, parameter := range operation.Parameters {
		if parameter.Value.In == parameterLocationHeader && parameter.Value.Name == name {
			return !required || parameter.Value.Required
		}
	}
	return false
}

// requireSingleValuedHeaders rejects a repeated declared header, which would otherwise let a caller
// smuggle a second value past a validator that reads only the first.
func requireSingleValuedHeaders(b *boundaryRequest) *contractError {
	for _, parameter := range b.route.Operation.Parameters {
		if parameter.Value.In != parameterLocationHeader {
			continue
		}
		if len(b.request.Header.Values(parameter.Value.Name)) > 1 {
			return &contractError{code: codeInvalidHeader, message: messageInvalidHeader}
		}
	}
	return nil
}
