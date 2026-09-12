package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"
	"github.com/go-chi/chi/v5/middleware"
)

// Transport error codes the boundary can answer with. Every specification declares the same error
// enum, so the health projection's generated constants are the single source for their spelling.
const (
	codeAuthenticationRequired         = servedapi.AUTHENTICATIONREQUIRED
	codeEmailAlreadyRegistered         = servedapi.EMAILALREADYREGISTERED
	codeInvalidCredentials             = servedapi.INVALIDCREDENTIALS
	codeBodyTooLarge                   = servedapi.BODYTOOLARGE
	codeCSRFInvalid                    = servedapi.CSRFINVALID
	codeIdempotencyKeyInvalid          = servedapi.IDEMPOTENCYKEYINVALID
	codeIdempotencyKeyRequired         = servedapi.IDEMPOTENCYKEYREQUIRED
	codeInternalAuthenticationRequired = servedapi.INTERNALAUTHENTICATIONREQUIRED
	codeInternalError                  = servedapi.INTERNALERROR
	codeInvalidCursor                  = servedapi.INVALIDCURSOR
	codeInvalidHeader                  = servedapi.INVALIDHEADER
	codeMalformedJSON                  = servedapi.MALFORMEDJSON
	codeMethodNotAllowed               = servedapi.METHODNOTALLOWED
	codeOriginNotAllowed               = servedapi.ORIGINNOTALLOWED
	codeResourceNotFound               = servedapi.RESOURCENOTFOUND
	codeServiceUnavailable             = servedapi.SERVICEUNAVAILABLE
	codeUnsupportedMediaType           = servedapi.UNSUPPORTEDMEDIATYPE
	codeValidationFailed               = servedapi.VALIDATIONFAILED
)

// Messages shared by more than one failure. A message specific to a single failure is spelled at
// the point of use instead.
const (
	messageAuthenticationRequired = "Authentication required"
	messageCSRFInvalid            = "Invalid CSRF token"
	messageOriginNotAllowed       = "Origin not allowed"
	messageInternalError          = "Internal server error"
	messageInvalidHeader          = "Invalid header"
	messageMalformedJSON          = "Malformed JSON"
	messageMethodNotAllowed       = "Method not allowed"
	messageResourceNotFound       = "Resource not found"
	messageValidationFailed       = "Request validation failed"
)

// transportErrorStatus is the status each transport code carries. The specifications declare one
// status per code, so deriving it here keeps every response consistent with the contract by
// construction. Domain failures are answered by generated response types rather than writeError
// and therefore do not appear here.
var transportErrorStatus = map[servedapi.ErrorCode]int{
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

// apiErrorBody renders the JSON error contract for a handler, which returns its response rather
// than writing one. writeError covers the boundary, where there is no generated response type yet.
func apiErrorBody(ctx context.Context, code servedapi.ErrorCode, message string) servedapi.ApiError {
	return servedapi.ApiError{Code: code, Message: message, RequestId: middleware.GetReqID(ctx)}
}

// apiError is a transport failure a boundary step reports instead of writing a response itself, so
// that a step stays independent of the response writer.
type apiError struct {
	code       servedapi.ErrorCode
	message    string
	violations []violation
}

// writeError emits the single JSON error contract. All four specifications declare the same
// envelope, so the health projection supplies its generated type for every router on this boundary.
func writeError(w http.ResponseWriter, r *http.Request,
	code servedapi.ErrorCode, message string, violations ...violation) {
	status, declared := transportErrorStatus[code]
	if !declared {
		status = http.StatusInternalServerError
	}
	body := servedapi.ApiError{Code: code, Message: message, RequestId: requestID(r)}
	if len(violations) > 0 {
		body.Details = violationDetails(violations)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeRequestError reports a failure raised by the specification router or by the generated
// request validator, which report an unroutable path and an invalid payload through one error.
func writeRequestError(spec *openapi3.T, w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, routers.ErrPathNotFound):
		writeError(w, r, codeResourceNotFound, messageResourceNotFound)
	case errors.Is(err, routers.ErrMethodNotAllowed):
		writeMethodNotAllowed(spec, w, r)
	default:
		writeValidationError(w, r, err)
	}
}

// writeMethodNotAllowed answers a path the specification declares but not for this method. A path
// whose operations were all removed from the router is not part of this API at all, so it stays a
// 404 instead of advertising methods the router will never accept.
func writeMethodNotAllowed(spec *openapi3.T, w http.ResponseWriter, r *http.Request) {
	declared := spec.Paths.Value(r.URL.Path)
	if declared == nil {
		writeError(w, r, codeMethodNotAllowed, messageMethodNotAllowed)
		return
	}
	allowed := allowedMethods(declared)
	if len(allowed) == 0 {
		writeError(w, r, codeResourceNotFound, messageResourceNotFound)
		return
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, r, codeMethodNotAllowed, messageMethodNotAllowed)
}

// allowedMethods lists a path's methods in a stable order so that the Allow header of an identical
// request never varies with map iteration.
func allowedMethods(path *openapi3.PathItem) []string {
	methods := make([]string, 0, len(path.Operations()))
	for method := range path.Operations() {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}

// writeValidationError reports an invalid payload or parameter. Positional constraints were
// collected before the validator ran, so they are merged in here to report one complete set of
// violations. A parameter the contract gives its own code is answered with that code instead.
func writeValidationError(w http.ResponseWriter, r *http.Request, err error) {
	violations := collectViolations(err, "body", "")
	if carried, ok := r.Context().Value(constraintsKey{}).([]violation); ok {
		violations = append(violations, carried...)
	}
	for _, v := range violations {
		if failure := parameterFailure(r, v); failure != nil {
			writeError(w, r, failure.code, failure.message)
			return
		}
	}
	writeError(w, r, codeValidationFailed, messageValidationFailed, violations...)
}

// parameterFailure maps a violation on a parameter the contract answers with a dedicated code, and
// reports nil when the parameter is covered by the generic validation response.
func parameterFailure(r *http.Request, v violation) *apiError {
	switch {
	case v.Location == "header":
		return headerFailure(r, v.Parameter)
	case v.Location == "query" && v.Parameter == "cursor":
		return &apiError{code: codeInvalidCursor, message: "Invalid cursor"}
	}
	return nil
}

// headerFailure separates an absent header from a malformed one. A request without Origin or
// X-CSRF-Token carries no allowed origin or token at all, which the contract rejects with 403,
// while a header that is present but malformed stays a malformed header.
func headerFailure(r *http.Request, name string) *apiError {
	absent := r.Header.Get(name) == ""
	switch name {
	case "Idempotency-Key":
		if absent {
			return &apiError{code: codeIdempotencyKeyRequired, message: messageInvalidHeader}
		}
		return &apiError{code: codeIdempotencyKeyInvalid, message: messageInvalidHeader}
	case "Origin":
		if absent {
			return &apiError{code: codeOriginNotAllowed, message: messageOriginNotAllowed}
		}
	case "X-CSRF-Token":
		if absent {
			return &apiError{code: codeCSRFInvalid, message: messageCSRFInvalid}
		}
	}
	return &apiError{code: codeInvalidHeader, message: messageInvalidHeader}
}
