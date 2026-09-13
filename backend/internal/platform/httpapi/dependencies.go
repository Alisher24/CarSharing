package httpapi

import (
	"errors"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dependencies is everything the served application needs from the process around it. The process
// that builds them supplies all of them; a caller that serves only what needs no account — an
// isolated contract router, or a routing test — supplies the probe and the catalog and is served
// by NewAnonymousRouter instead.
type Dependencies struct {
	// Probe answers whether the dependencies this deployment needs are usable. It is the only
	// dependency the health operations have.
	Probe ReadinessProbe

	// AllowedOrigins are the browser origins a mutation may come from. An origin outside this set
	// is refused before the request can create an account, a session or a cookie.
	AllowedOrigins []string

	Pool     *pgxpool.Pool
	Sessions *sessions.Manager
	Auth     *auth.Service
	Users    *auth.UserStore
	Throttle *auth.Throttle

	// Catalog is what the operations that need no account read. Both routers are given it,
	// because the anonymous one serves those operations too.
	Catalog Catalog
}

// ErrIncompleteApplication refuses to serve an application whose dependencies were not all
// supplied. A service locator that answers a missing dependency at request time panics inside a
// handler instead, which a client reads as a crash rather than as a process that never started.
var ErrIncompleteApplication = errors.New("the HTTP application is missing a dependency")

// server implements every served operation by delegating to the handler that owns its concern, so
// that the generated interface is satisfied in one place without collecting unrelated methods on
// one type.
type server struct {
	health
	accounts
	catalogHandlers
}
