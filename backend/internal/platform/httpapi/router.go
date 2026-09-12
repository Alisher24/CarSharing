package httpapi

import (
	"net/http"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Application is everything the HTTP layer needs from the process around it. The health operations
// need only the readiness probe, so a caller that serves nothing else may leave the rest unset.
type Application struct {
	Probe ReadinessProbe

	// AllowedOrigins are the browser origins a mutation may come from. An origin outside this set
	// is refused before the request can create an account, a session or a cookie.
	AllowedOrigins []string

	Pool     *pgxpool.Pool
	Sessions *sessions.Manager
	Auth     *auth.Service
	Users    *auth.UserStore
	Throttle *auth.Throttle
}

// server implements every served operation by delegating to the handler that owns its concern, so
// that the generated interface is satisfied in one place without collecting unrelated methods on
// one type.
type server struct {
	health
	accounts
}

// Router serves the operations this application implements. The generated projection also declares
// the planned operations, but with no operation left on their paths, so those paths are dropped
// from the specification before routing and answer as an unknown resource rather than as a method
// that exists but is not allowed.
func Router(app Application) http.Handler {
	spec, err := servedapi.GetSwagger()
	if err != nil {
		panic(err)
	}
	for path, pathItem := range spec.Paths.Map() {
		if len(pathItem.Operations()) == 0 {
			spec.Paths.Delete(path)
		}
	}
	options := servedapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, r, codeMalformedJSON, messageMalformedJSON)
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, r, codeInternalError, messageInternalError)
		},
	}
	implementation := server{
		health: health{probe: app.Probe},
		accounts: accounts{
			pool: app.Pool, sessions: app.Sessions, service: app.Auth, users: app.Users,
			throttle: app.Throttle,
		},
	}
	handler := servedapi.NewStrictHandlerWithOptions(implementation, nil, options)
	// The session is attached before the boundary so that the boundary's credential check and the
	// handler below it read one resolved session rather than querying the store twice.
	policy := transport{allowedOrigins: originSet(app.AllowedOrigins), authenticate: authenticateSession}
	return withClientAddress(withSession(app.Sessions, boundary(spec, servedapi.Handler(handler), policy)))
}

// originSet indexes the allowed origins for lookup, so the check is a comparison rather than a scan.
func originSet(origins []string) map[string]bool {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return allowed
}
