// Package ratelimit counts attempts per subject in PostgreSQL so that guessing a password costs an
// attacker time. Counters are deliberately kept outside the transaction of the attempt they count:
// a refused sign-in rolls its transaction back, and a counter that rolled back with it would leave
// the attempt free.
package ratelimit

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
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

// Reached reports a limit that is currently exhausted, and how long until it recovers. The wait is
// what the caller advertises in Retry-After, so it is the real remaining time rather than the whole
// window.
type Reached struct {
	Scope Scope
	Wait  time.Duration
}

// Counter records attempts and reports which limits are exhausted.
type Counter struct {
	pool   *pgxpool.Pool
	limits map[Scope]Limit
}

func NewCounter(pool *pgxpool.Pool, limits map[Scope]Limit) *Counter {
	return &Counter{pool: pool, limits: limits}
}

// Limit reports the configured limit for a scope, so a caller can state what it enforces.
func (c *Counter) Limit(scope Scope) Limit { return c.limits[scope] }

// Counted is one subject to check or record under a scope.
type Counted struct {
	Scope   Scope
	Subject string
}

// Exhausted reports the first of the given subjects whose limit is already reached, without
// recording anything. It is called before a password is verified, so a throttled attempt never
// pays for a hash. The caller passes the subjects in the order it wants them reported, so two
// identical requests always learn about the same limit and advertise the same wait.
func (c *Counter) Exhausted(ctx context.Context, subjects []Counted) (Reached, bool, error) {
	for _, counted := range subjects {
		limit, configured := c.limits[counted.Scope]
		if !configured {
			continue
		}
		attempts, windowStartedAt, found, err := c.read(ctx, counted.Scope, counted.Subject, limit.Window)
		if err != nil {
			return Reached{}, false, err
		}
		if found && attempts >= limit.Attempts {
			return Reached{Scope: counted.Scope, Wait: remaining(windowStartedAt, limit.Window)}, true, nil
		}
	}
	return Reached{}, false, nil
}

// Record adds one attempt to a subject's counter, starting a new window when the previous one has
// passed. A window that has passed is simply replaced, which is what makes access return on its
// own without any unblocking step.
func (c *Counter) Record(ctx context.Context, scope Scope, subject string) error {
	limit, configured := c.limits[scope]
	if !configured {
		return nil
	}
	_, err := c.pool.Exec(ctx, `
		INSERT INTO rate_limit_counters (scope, subject, window_started_at, attempts)
		VALUES ($1, $2, now(), 1)
		ON CONFLICT (scope, subject) DO UPDATE SET
			window_started_at = CASE
				WHEN rate_limit_counters.window_started_at + $3::interval <= now()
				THEN now() ELSE rate_limit_counters.window_started_at END,
			attempts = CASE
				WHEN rate_limit_counters.window_started_at + $3::interval <= now()
				THEN 1 ELSE rate_limit_counters.attempts + 1 END`,
		string(scope), subject, limit.Window)
	return err
}

func (c *Counter) read(
	ctx context.Context, scope Scope, subject string, window time.Duration,
) (int, time.Time, bool, error) {
	var attempts int
	var windowStartedAt time.Time
	err := c.pool.QueryRow(ctx, `
		SELECT attempts, window_started_at FROM rate_limit_counters
		WHERE scope = $1 AND subject = $2 AND window_started_at + $3::interval > now()`,
		string(scope), subject, window).Scan(&attempts, &windowStartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, time.Time{}, false, nil
	}
	if err != nil {
		return 0, time.Time{}, false, err
	}
	return attempts, windowStartedAt, true, nil
}

// remaining is the wait until a window ends, never below a second so that a caller told to wait is
// not immediately invited back by a rounded-down zero.
func remaining(windowStartedAt time.Time, window time.Duration) time.Duration {
	wait := time.Until(windowStartedAt.Add(window))
	if wait < time.Second {
		return time.Second
	}
	return wait.Round(time.Second)
}
