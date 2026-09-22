package httpapi

import (
	"context"
	"net/http"

	"github.com/Alisher24/CarSharing/backend/internal/platform/httpheader"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// withRequestIdentity assigns the request an identifier and echoes it on the response together with
// the no-store policy every API response carries. It runs before routing so that an error written
// for an unroutable path still reports an identifier a client can quote.
func withRequestIdentity(w http.ResponseWriter, r *http.Request) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), middleware.RequestIDKey, uuid.NewString()))
	w.Header().Set(httpheader.RequestID, requestID(r))
	w.Header().Set(cacheControlHeader, noStoreCacheControl)
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
