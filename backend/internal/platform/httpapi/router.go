package httpapi

import (
	"net/http"

	healthapi "github.com/Alisher24/CarSharing/backend/internal/contracts/healthapi"
)

// Router serves the operations this application implements. The generated projection also declares
// the planned operations, but with no operation left on their paths, so those paths are dropped
// from the specification before routing and answer as an unknown resource rather than as a method
// that exists but is not allowed.
func Router(probe ReadinessProbe) http.Handler {
	spec, err := healthapi.GetSwagger()
	if err != nil {
		panic(err)
	}
	for path, item := range spec.Paths.Map() {
		if len(item.Operations()) == 0 {
			spec.Paths.Delete(path)
		}
	}
	options := healthapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, r, codeMalformedJSON, messageMalformedJSON)
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, r, codeInternalError, messageInternalError)
		},
	}
	handler := healthapi.NewStrictHandlerWithOptions(health{probe: probe}, nil, options)
	return boundary(spec, healthapi.Handler(handler))
}
