package sessions

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CookieName is the only place the session token is carried. It is never part of a JSON body, so a
// script that reads a response learns nothing it could replay.
const CookieName = "carsharing_session"

// Lifetime is how long a session lasts from the moment it is created. It is absolute: a request
// made with a live session does not push the expiry further out, so a stolen token stops working
// at a time its holder cannot extend.
const Lifetime = 12 * time.Hour

// Keys the session payload is stored under. They are internal to this package: no caller reads the
// payload directly, so a snapshot is the only way to learn who a session belongs to.
const (
	userIDKey    = "user_id"
	csrfTokenKey = "csrf_token"
)

// ErrSessionPayloadUnusable reports a stored session whose payload does not describe a user. It is
// a storage defect rather than an expired session, so a caller answers it as a service failure
// instead of quietly signing the person out.
var ErrSessionPayloadUnusable = errors.New("stored session does not identify a user")

// Snapshot is what a live session tells the application: whose it is, the CSRF token issued with
// it, and when it expires.
type Snapshot struct {
	UserID    uuid.UUID
	CSRFToken string
	ExpiresAt time.Time
}

// Issued is a session that has just been created, together with the token the browser must carry.
// The token leaves the process only inside a Set-Cookie header.
type Issued struct {
	Snapshot Snapshot
	token    string
}

// Manager creates, restores and revokes sessions. It hides the session library's context handling
// so that the rest of the application works with whole sessions rather than with a token, a
// payload and a deadline carried separately.
type Manager struct {
	sessions     *scs.SessionManager
	secureCookie bool
}

// NewManager builds the session manager over PostgreSQL. The caller decides whether the cookie
// carries Secure, because the documented local profile serves plain HTTP while any other
// deployment is over HTTPS.
func NewManager(pool *pgxpool.Pool, secureCookie bool) *Manager {
	sessions := scs.New()
	sessions.Store = store{pool: pool}
	sessions.Lifetime = Lifetime
	// No idle timeout: the absolute lifetime is the only expiry, so activity never renews a session.
	sessions.IdleTimeout = 0
	sessions.HashTokenInStore = true
	return &Manager{sessions: sessions, secureCookie: secureCookie}
}

// Establish creates a session for a user and stores it through the context's querier, so a caller
// running inside a transaction commits the session exactly when it commits the rest of its work.
// The CSRF token is drawn per session and therefore never outlives the session that issued it.
func (m *Manager) Establish(ctx context.Context, userID uuid.UUID) (Issued, error) {
	sessionCtx, err := m.sessions.Load(ctx, "")
	if err != nil {
		return Issued{}, err
	}
	csrfToken, err := newCSRFToken()
	if err != nil {
		return Issued{}, err
	}
	m.sessions.Put(sessionCtx, userIDKey, userID.String())
	m.sessions.Put(sessionCtx, csrfTokenKey, csrfToken)
	token, expiresAt, err := m.sessions.Commit(sessionCtx)
	if err != nil {
		return Issued{}, err
	}
	snapshot := Snapshot{UserID: userID, CSRFToken: csrfToken, ExpiresAt: expiresAt}
	return Issued{Snapshot: snapshot, token: token}, nil
}

// Restore reports the session a token identifies. An absent, unknown or expired token is reported
// as not found rather than as an error, because all three leave the caller signed out.
func (m *Manager) Restore(ctx context.Context, token string) (Snapshot, bool, error) {
	if token == "" {
		return Snapshot{}, false, nil
	}
	sessionCtx, err := m.sessions.Load(ctx, token)
	if err != nil {
		return Snapshot{}, false, err
	}
	// A token the store did not find yields a fresh, empty session rather than a failure, so an
	// absent user is how an unknown or expired token reports itself.
	storedID := m.sessions.GetString(sessionCtx, userIDKey)
	if storedID == "" {
		return Snapshot{}, false, nil
	}
	userID, err := uuid.Parse(storedID)
	if err != nil {
		return Snapshot{}, false, ErrSessionPayloadUnusable
	}
	snapshot := Snapshot{
		UserID:    userID,
		CSRFToken: m.sessions.GetString(sessionCtx, csrfTokenKey),
		ExpiresAt: m.sessions.Deadline(sessionCtx),
	}
	return snapshot, true, nil
}

// Revoke removes one session. Revoking a token that is already gone succeeds, so a client may
// retry a sign-out it never saw the answer to.
func (m *Manager) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	sessionCtx, err := m.sessions.Load(ctx, token)
	if err != nil {
		return err
	}
	return m.sessions.Destroy(sessionCtx)
}

// Cookie is the Set-Cookie value that hands a browser a session.
func (m *Manager) Cookie(issued Issued) string {
	return m.cookie(issued.token, issued.Snapshot.ExpiresAt).String()
}

// ClearedCookie is the Set-Cookie value that removes the session cookie from a browser. It must
// carry the same attributes as the cookie it replaces, or the browser keeps the original.
func (m *Manager) ClearedCookie() string {
	cleared := m.cookie("", time.Unix(0, 0).UTC())
	cleared.MaxAge = -1
	return cleared.String()
}

func (m *Manager) cookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.secureCookie,
	}
}
