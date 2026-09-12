package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
	"github.com/getkin/kin-openapi/openapi3filter"
)

// Errors the session boundary distinguishes. An absent session is not among them: a caller who is
// simply signed out is reported as not live rather than as a failure.
var (
	// errSessionStoreUnavailable reports that the session behind a request could not be read at
	// all. It is kept distinct from an absent session because a store that cannot answer must not
	// be reported as a caller who is simply signed out: the contract answers the first with 503
	// and the second with 401.
	errSessionStoreUnavailable = errors.New("session store did not answer")

	// errNoLiveSession reports a request that presented no usable session credential.
	errNoLiveSession = errors.New("request carries no live session")
)

// sessionContextKey addresses the session of the request being served.
type sessionContextKey struct{}

// requestSession resolves the session a request arrived with, at most once. The strict handlers
// receive a context rather than the request, so the cookie is read here and every later reader —
// the contract's authentication step and the handler itself — shares this one answer instead of
// querying the store again.
type requestSession struct {
	manager  *sessions.Manager
	token    string
	once     sync.Once
	snapshot sessions.Snapshot
	live     bool
	err      error
}

// resolve reports the session this request carries. An absent, unknown or expired token is not an
// error: it is a caller who is not signed in.
func (s *requestSession) resolve(ctx context.Context) (sessions.Snapshot, bool, error) {
	// A router assembled without a session store — the isolated contract routers — serves no
	// session-bearing operation, so a request through it is simply not signed in. A store that is
	// present but fails is a different matter and is reported below as the error it is.
	if s == nil || s.manager == nil {
		return sessions.Snapshot{}, false, nil
	}
	s.once.Do(func() { s.snapshot, s.live, s.err = s.manager.Restore(ctx, s.token) })
	return s.snapshot, s.live, s.err
}

// withSession attaches the unresolved session to every request. Resolution is deferred to the
// first reader, so an operation that needs no session — liveness above all — never queries the
// store.
func withSession(manager *sessions.Manager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := requestSessionOf(r, manager)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, current)))
	})
}

// requestSessionOf names the session a request arrives with, without resolving it yet.
func requestSessionOf(r *http.Request, manager *sessions.Manager) *requestSession {
	current := &requestSession{manager: manager}
	if cookie, err := r.Cookie(sessions.CookieName); err == nil {
		current.token = cookie.Value
	}
	return current
}

func sessionOf(ctx context.Context) *requestSession {
	current, _ := ctx.Value(sessionContextKey{}).(*requestSession)
	return current
}

// sessionToken returns the token the request arrived with, or the empty string when it carried no
// session cookie. Operations that replace or revoke a session need the token itself rather than
// the session it names.
func sessionToken(ctx context.Context) string {
	if current := sessionOf(ctx); current != nil {
		return current.token
	}
	return ""
}

// authenticateSession satisfies the contract's Session security scheme. It reports the store's own
// failure separately, so that a request whose credentials could not be checked is refused as a
// service failure rather than waved through or mistaken for a signed-out caller.
func authenticateSession(ctx context.Context, input *openapi3filter.AuthenticationInput) error {
	_, live, err := sessionOf(input.RequestValidationInput.Request.Context()).resolve(ctx)
	if err != nil {
		return errSessionStoreUnavailable
	}
	if !live {
		return errNoLiveSession
	}
	return nil
}
