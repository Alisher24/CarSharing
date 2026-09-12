package httpapi

import (
	"net/http"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
)

// errorStatus is the status each transport code carries. The specifications declare one status per
// code, so deriving it here keeps every response consistent with the contract by construction.
// Domain failures are answered by generated response types rather than writeError and therefore do
// not appear here.
var errorStatus = map[servedapi.ErrorCode]int{
	codeAuthenticationRequired:         http.StatusUnauthorized,
	codeBodyTooLarge:                   http.StatusRequestEntityTooLarge,
	codeCSRFInvalid:                    http.StatusForbidden,
	codeIdempotencyKeyInvalid:          http.StatusBadRequest,
	codeIdempotencyKeyRequired:         http.StatusBadRequest,
	codeInternalAuthenticationRequired: http.StatusUnauthorized,
	codeInternalError:                  http.StatusInternalServerError,
	codeInvalidCursor:                  http.StatusBadRequest,
	codeInvalidHeader:                  http.StatusBadRequest,
	codeMalformedJSON:                  http.StatusBadRequest,
	codeMethodNotAllowed:               http.StatusMethodNotAllowed,
	codeOriginNotAllowed:               http.StatusForbidden,
	codeResourceNotFound:               http.StatusNotFound,
	codeServiceUnavailable:             http.StatusServiceUnavailable,
	codeUnsupportedMediaType:           http.StatusUnsupportedMediaType,
	codeValidationFailed:               http.StatusUnprocessableEntity,
}
