// Package ratelimit counts attempts per subject in PostgreSQL so that guessing a password costs an
// attacker time. Counters are deliberately kept outside the transaction of the attempt they count:
// a refused sign-in rolls its transaction back, and a counter that rolled back with it would leave
// the attempt free. An attempt is counted by the same statement that decides whether it fits, so a
// burst of simultaneous attempts cannot each be told that the budget still has room.
package ratelimit

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Scope names what a counter counts. The scope and the subject together address one counter, so a
// person's own address and their IP are limited independently.
type Scope string

// Limit is how many attempts a subject may make within one window.
type Limit struct {
	Attempts int
	Window   time.Duration
}

// Limits is the configured budget of each counted operation the platform knows about. A feature
// reads the budget it counts under from here and pairs it with the scope it counts against, so the
// loader and the feature cannot disagree about which settings exist.
type Limits struct {
	SignInByEmailAndAddress Limit
	SignInByEmail           Limit
	SignInByAddress         Limit
	RegistrationByAddress   Limit
}

// Reached reports a limit that is currently exhausted, and how long until it recovers. The wait is
// what the caller advertises in Retry-After, so it is the real remaining time rather than the whole
// window.
type Reached struct {
	// Refused reports that the limit refused the attempt rather than granting it. It is stated
	// rather than derived from the scope, because a granted claim names no scope either.
	Refused bool
	Scope   Scope
	Wait    time.Duration
}

// Fits reports whether the attempt the counter was asked about was granted rather than refused.
func (r Reached) Fits() bool { return !r.Refused }

// Counter spends attempts against subjects and reports which limits refuse them.
type Counter struct {
	pool   *pgxpool.Pool
	limits map[Scope]Limit
}

func NewCounter(pool *pgxpool.Pool, limits map[Scope]Limit) *Counter {
	return &Counter{pool: pool, limits: limits}
}

// Counted is one subject to spend an attempt against under a scope.
type Counted struct {
	Scope   Scope
	Subject string
}

// claimStatement adds one attempt to a subject's counter and reports the count it stands at. It is
// one statement so that the decision and the increment cannot be separated: two round trips would let
// a burst of simultaneous attempts decide against the same count and each proceed, which would
// release more password checks than the budget allows.
//
// A row whose window has already passed starts a fresh window holding the attempt rather than being
// incremented, which is what makes access return on its own. A row that does not fit keeps the count
// it had, so a refused attempt is never counted, and the moment its window began comes back with it
// so the caller can advertise the real remaining wait.
const claimStatement = `
INSERT INTO rate_limit_counters (scope, subject, window_started_at, attempts)
VALUES ($1, $2, now(), 1)
ON CONFLICT (scope, subject) DO UPDATE SET
	window_started_at = CASE
		WHEN rate_limit_counters.window_started_at + $3::interval <= now()
		THEN now() ELSE rate_limit_counters.window_started_at END,
	attempts = CASE
		WHEN rate_limit_counters.window_started_at + $3::interval <= now()
		THEN 1
		WHEN rate_limit_counters.attempts < $4
		THEN rate_limit_counters.attempts + 1
		ELSE rate_limit_counters.attempts END
RETURNING attempts, window_started_at, window_started_at + $3::interval <= now() AS window_expired`

// Claim spends one attempt of a subject's budget and reports whether it fit. It is the only way an
// attempt is counted: the caller learns from the same statement whether the limit still allows the
// work the attempt is about to do, so a burst of simultaneous attempts cannot each be told that the
// budget has room.
func (c *Counter) Claim(ctx context.Context, scope Scope, subject string) (Reached, error) {
	limit, configured := c.limits[scope]
	if !configured {
		return Reached{}, nil
	}
	var attempts int
	var windowStartedAt time.Time
	// The window's state is not read back: the count alone decides whether the attempt fit, and the
	// statement is the one place that must know whether it was counting in a fresh window.
	var windowExpired bool
	err := c.pool.QueryRow(ctx, claimStatement, string(scope), subject, limit.Window, limit.Attempts).
		Scan(&attempts, &windowStartedAt, &windowExpired)
	if err != nil {
		return Reached{}, err
	}
	// The statement keeps the count it had when it did not fit, so a count that has reached the limit
	// is a refused attempt rather than a spent one.
	if attempts >= limit.Attempts {
		return Reached{Refused: true, Scope: scope, Wait: remaining(windowStartedAt, limit.Window)}, nil
	}
	return Reached{}, nil
}

// Release gives back an attempt a subject's counter was charged for. A caller that claims the budget
// of a failure before it knows the outcome releases it when the outcome turns out not to be a
// failure, so a counter that records failures keeps recording failures and never a correct attempt.
//
// A counter whose last attempt is given back is removed rather than left at zero: an attempt is the
// whole of what a counter holds, and a row stating that nothing was attempted is a second way of
// saying the row is not there. Releasing a subject whose counter is already gone changes nothing, so
// a caller may release an attempt the reaper has already removed.
func (c *Counter) Release(ctx context.Context, scope Scope, subject string) error {
	if _, configured := c.limits[scope]; !configured {
		return nil
	}
	_, err := c.pool.Exec(ctx, `
		UPDATE rate_limit_counters SET attempts = attempts - 1
		WHERE scope = $1 AND subject = $2 AND attempts > 1`,
		string(scope), subject)
	if err != nil {
		return err
	}
	_, err = c.pool.Exec(ctx, `
		DELETE FROM rate_limit_counters WHERE scope = $1 AND subject = $2 AND attempts <= 1`,
		string(scope), subject)
	return err
}

// MinimumRetryAfter is the shortest wait a caller is ever told to wait. A rounded-down zero would
// invite the caller straight back, so every wait this package reports is floored here.
const MinimumRetryAfter = time.Second

// remaining is the wait until a window ends, never below MinimumRetryAfter.
func remaining(windowStartedAt time.Time, window time.Duration) time.Duration {
	wait := time.Until(windowStartedAt.Add(window))
	if wait < MinimumRetryAfter {
		return MinimumRetryAfter
	}
	return wait.Round(time.Second)
}
