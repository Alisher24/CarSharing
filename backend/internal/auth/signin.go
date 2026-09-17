package auth

import (
	"context"
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/platform/ratelimit"
)

// LimitReachedError reports an attempt that a limit refused. It carries the limit that refused it and
// how long it has left, which is what the caller advertises as the wait before another attempt.
type LimitReachedError struct{ Reached ratelimit.Reached }

func (e *LimitReachedError) Error() string {
	return fmt.Sprintf("limit %s is exhausted for another %s", e.Reached.Scope, e.Reached.Wait)
}

// SignInAttempt is the budget a sign-in spent before its password was verified, and what became of
// it. The attempt is claimed from every sign-in limit, so the claim itself bounds how many
// simultaneous attempts can reach a password; a sign-in that turns out to be correct gives back the
// limits that count failures, which is what keeps them counting failures and not correct attempts.
type SignInAttempt struct{ refundable []ratelimit.Counted }

// ClaimSignIn spends one attempt from every sign-in limit and reports the one that refused it, or an
// error when the limits could not be consulted at all. A refused claim has already spent the narrower
// limits it passed, which is deliberate: the request was made, and an attacker who is refused one
// limit must not be handed back the budget of another.
//
// The error is a *LimitReachedError when a budget refused the attempt and the database's own failure
// otherwise, so a caller can answer a refusal as a refusal and a broken service as a broken service.
func (t *Throttle) ClaimSignIn(
	ctx context.Context, email Email, clientAddress string,
) (SignInAttempt, error) {
	counter, err := t.counterOf()
	if err != nil {
		return SignInAttempt{}, err
	}
	attempt := SignInAttempt{refundable: refundableSignInSubjects(email, clientAddress)}
	for _, counted := range signInSubjects(email, clientAddress) {
		reached, err := counter.Claim(ctx, counted.Scope, counted.Subject)
		if err != nil {
			return SignInAttempt{}, err
		}
		if !reached.Fits() {
			return SignInAttempt{}, &LimitReachedError{Reached: reached}
		}
	}
	return attempt, nil
}

// Succeeded gives back the limits that record failures. A wrong guess needs nothing recorded, because
// the claim already spent them; a correct one still spends the address limit, which counts every
// attempt from an address whether or not it is right.
func (a SignInAttempt) Succeeded(ctx context.Context, t *Throttle) error {
	counter, err := t.counterOf()
	if err != nil {
		return err
	}
	for _, counted := range a.refundable {
		if err = counter.Release(ctx, counted.Scope, counted.Subject); err != nil {
			return err
		}
	}
	return nil
}
