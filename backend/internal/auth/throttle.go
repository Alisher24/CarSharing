package auth

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/ratelimit"
)

// The counted scopes: an email-scoped counter records failures only, so a person signing in
// correctly never spends their own budget; an address-scoped counter records every attempt, which
// is what bounds a spray across many addresses from one source.
const (
	SignInByEmailAndAddress ratelimit.Scope = "sign_in_email_address"
	SignInByEmail           ratelimit.Scope = "sign_in_email"
	SignInByAddress         ratelimit.Scope = "sign_in_address"
	RegistrationByAddress   ratelimit.Scope = "registration_address"
)

// ErrThrottleUnavailable reports that the limits could not be consulted. A caller answers it as a
// service failure: a service that cannot tell whether an attempt is within its limits must not
// grant an unlimited number of guesses instead.
var ErrThrottleUnavailable = errors.New("rate limits cannot be consulted")

// Throttle decides whether an attempt may proceed and records what it costs. It is consulted
// before a password is verified, so an attempt that is already over a limit never pays for a hash.
type Throttle struct{ counter *ratelimit.Counter }

func NewThrottle(counter *ratelimit.Counter) *Throttle { return &Throttle{counter: counter} }

// counterOf reports the counter to record against, or the error to answer when the throttle itself
// is missing. A nil throttle is how a process assembled without rate limits reports that it cannot
// decide anything, rather than reading it as an unlimited budget.
func (t *Throttle) counterOf() (*ratelimit.Counter, error) {
	if t == nil || t.counter == nil {
		return nil, ErrThrottleUnavailable
	}
	return t.counter, nil
}

// SignInAllowed reports whether a sign-in may be attempted, and how long to wait when it may not.
// All three sign-in limits are checked: the pair, the address alone and the email alone.
func (t *Throttle) SignInAllowed(
	ctx context.Context, email Email, clientAddress string,
) (time.Duration, bool, error) {
	return t.allowed(ctx, signInSubjects(email, clientAddress))
}

// RegistrationAllowed reports whether a registration may be attempted from an address.
func (t *Throttle) RegistrationAllowed(
	ctx context.Context, clientAddress string,
) (time.Duration, bool, error) {
	return t.allowed(ctx, []ratelimit.Counted{
		{Scope: RegistrationByAddress, Subject: clientAddress},
	})
}

// RecordSignInAttempt counts an attempt against the address it came from. Every sign-in costs the
// source its budget, whether or not the credentials turn out to be right.
func (t *Throttle) RecordSignInAttempt(ctx context.Context, clientAddress string) error {
	counter, err := t.counterOf()
	if err != nil {
		return err
	}
	return counter.Record(ctx, SignInByAddress, clientAddress)
}

// RecordSignInFailure counts a wrong guess against the address being guessed at. It is recorded
// outside the transaction of the refused attempt, which would otherwise roll the counter back and
// leave the guess free.
func (t *Throttle) RecordSignInFailure(ctx context.Context, email Email, clientAddress string) error {
	counter, err := t.counterOf()
	if err != nil {
		return err
	}
	if err = counter.Record(ctx, SignInByEmailAndAddress, emailAndAddress(email, clientAddress)); err != nil {
		return err
	}
	return counter.Record(ctx, SignInByEmail, string(email))
}

// RecordRegistrationAttempt counts a registration against the address it came from.
func (t *Throttle) RecordRegistrationAttempt(ctx context.Context, clientAddress string) error {
	counter, err := t.counterOf()
	if err != nil {
		return err
	}
	return counter.Record(ctx, RegistrationByAddress, clientAddress)
}

func (t *Throttle) allowed(
	ctx context.Context, subjects []ratelimit.Counted,
) (time.Duration, bool, error) {
	counter, err := t.counterOf()
	if err != nil {
		return 0, false, err
	}
	reached, exhausted, err := counter.Exhausted(ctx, subjects)
	if err != nil {
		return 0, false, err
	}
	if exhausted {
		return reached.Wait, false, nil
	}
	return 0, true, nil
}

// signInSubjects lists the sign-in limits in the order a refusal reports them: the narrowest
// first, so a person whose own pair is exhausted is told about that rather than about a limit an
// unrelated caller filled.
func signInSubjects(email Email, clientAddress string) []ratelimit.Counted {
	return []ratelimit.Counted{
		{Scope: SignInByEmailAndAddress, Subject: emailAndAddress(email, clientAddress)},
		{Scope: SignInByEmail, Subject: string(email)},
		{Scope: SignInByAddress, Subject: clientAddress},
	}
}

// emailAndAddress joins the pair with a separator no canonical email can contain, so two different
// pairs cannot collide into one counter.
func emailAndAddress(email Email, clientAddress string) string {
	return string(email) + " " + clientAddress
}
