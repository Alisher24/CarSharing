package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/go-chi/chi/v5/middleware"
)

// contractError is a transport failure a boundary step reports instead of writing a response
// itself, so that a step stays independent of the response writer.
type contractError struct {
	code       servedapi.ErrorCode
	message    string
	violations []violation
}

// apiErrorBody renders the JSON error contract for a handler, which returns its response rather
// than writing one. writeError covers the boundary, where there is no generated response type yet.
func apiErrorBody(ctx context.Context, code servedapi.ErrorCode, message string) servedapi.ApiError {
	return servedapi.ApiError{Code: code, Message: message, RequestId: middleware.GetReqID(ctx)}
}

// serviceUnavailable is the body of every operation's 503. Each operation declares its own 503
// shape, so the body is built here and wrapped at the point of use.
func serviceUnavailable(ctx context.Context) servedapi.ApiError {
	return apiErrorBody(ctx, codeServiceUnavailable, messageServiceUnavailable)
}

// loginUnavailable is the sign-in 503, which more than one step of the operation answers with.
func loginUnavailable(ctx context.Context) servedapi.Login503JSONResponse {
	return servedapi.Login503JSONResponse{Body: serviceUnavailable(ctx)}
}

// writeError emits the single JSON error contract. All four specifications declare the same
// envelope, so the health projection supplies its generated type for every router on this boundary.
func writeError(w http.ResponseWriter, r *http.Request,
	code servedapi.ErrorCode, message string, violations ...violation) {
	status, declared := errorStatus[code]
	if !declared {
		status = http.StatusInternalServerError
	}
	body := servedapi.ApiError{Code: code, Message: message, RequestId: requestID(r)}
	if len(violations) > 0 {
		body.Details = violationDetails(violations)
	}
	w.Header().Set(contentTypeHeader, jsonMediaType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
