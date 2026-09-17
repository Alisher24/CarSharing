package auth

import (
	"errors"

	"github.com/Alisher24/CarSharing/backend/internal/platform/ratelimit"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The counted scopes. What each one counts is what makes the budget mean something: a scope that
// counts failures bounds guessing at one identity, a scope that counts every attempt bounds a spray
// from one source whether or not any guess is right.
const (
	SignInByEmailAndAddress ratelimit.Scope = "sign_in_email_address"
	SignInByEmail           ratelimit.Scope = "sign_in_email"
	SignInByAddress         ratelimit.Scope = "sign_in_address"
	RegistrationByAddress   ratelimit.Scope = "registration_address"
)

// rateLimits is the budget of each counted scope. It is the platform's settings paired with the
// scopes this package counts under, so a scope cannot be added without its budget beside it.
type rateLimits map[ratelimit.Scope]ratelimit.Limit

// ErrThrottleUnavailable reports that the limits could not be consulted. A caller answers it as a
// service failure: a service that cannot tell whether an attempt is within its limits must not
// grant an unlimited number of guesses instead.
var ErrThrottleUnavailable = errors.New("rate limits cannot be consulted")

// Throttle decides whether an attempt may proceed and spends what it costs. It is consulted before a
// password is verified, and the answer it gives is the decision of one database statement rather
// than of a check that a later record could overtake, so a burst of simultaneous attempts cannot all
// be told that the budget has room.
type Throttle struct{ counter *ratelimit.Counter }

// NewThrottle counts attempts in PostgreSQL under the scopes this package names. The scopes and the
// budgets they are configured with are paired here rather than at the process root, so a new
// counted scope cannot be added with its budget wired up nowhere.
func NewThrottle(pool *pgxpool.Pool, limits ratelimit.Limits) *Throttle {
	return &Throttle{counter: ratelimit.NewCounter(pool, rateLimits{
		SignInByEmailAndAddress: limits.SignInByEmailAndAddress,
		SignInByEmail:           limits.SignInByEmail,
		SignInByAddress:         limits.SignInByAddress,
		RegistrationByAddress:   limits.RegistrationByAddress,
	})}
}

// counterOf reports the counter to spend against, or the error to answer when the throttle itself is
// missing. A nil throttle is how a process assembled without rate limits reports that it cannot
// decide anything, rather than reading it as an unlimited budget.
func (t *Throttle) counterOf() (*ratelimit.Counter, error) {
	if t == nil || t.counter == nil {
		return nil, ErrThrottleUnavailable
	}
	return t.counter, nil
}

// signInSubjects lists every sign-in limit one attempt is counted against, the narrowest first, so
// that the order a refusal is reported in is a property of this one list.
func signInSubjects(email Email, clientAddress string) []ratelimit.Counted {
	return []ratelimit.Counted{
		{Scope: SignInByEmailAndAddress, Subject: emailAndAddress(email, clientAddress)},
		{Scope: SignInByEmail, Subject: string(email)},
		{Scope: SignInByAddress, Subject: clientAddress},
	}
}

// refundableSignInSubjects lists the sign-in limits a correct sign-in gives back: the two that record
// failures rather than attempts. The address limit is last in signInSubjects because it counts every
// attempt, so the split is stated once, by position, rather than as a second copy of the list.
func refundableSignInSubjects(email Email, clientAddress string) []ratelimit.Counted {
	subjects := signInSubjects(email, clientAddress)
	return subjects[:len(subjects)-1]
}

// emailAndAddress joins the pair with a separator no canonical email can contain, so two different
// pairs cannot collide into one counter.
func emailAndAddress(email Email, clientAddress string) string {
	return string(email) + " " + clientAddress
}
