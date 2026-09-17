package auth

import (
	"context"
	"time"
)

// ErrRegistrationRefused reports a registration that the address's budget refused. The wait it
// carries is the one the caller advertises, so a registration refused under load is told the same
// thing an exhausted sign-in is.
type ErrRegistrationRefused struct{ Wait time.Duration }

func (e *ErrRegistrationRefused) Error() string {
	return "registration is refused for another " + e.Wait.String()
}

// ClaimRegistration spends one attempt of the address's registration budget and reports the wait when
// the budget refuses it. The attempt is counted by the same statement that decides it, so a burst of
// simultaneous registrations cannot each be told that the budget has room.
//
// The error is an *ErrRegistrationRefused when the budget refused the attempt and the database's own
// failure otherwise, so a caller can answer a refusal as a refusal and a broken service as a broken
// service.
func (t *Throttle) ClaimRegistration(ctx context.Context, clientAddress string) error {
	counter, err := t.counterOf()
	if err != nil {
		return err
	}
	reached, err := counter.Claim(ctx, RegistrationByAddress, clientAddress)
	if err != nil {
		return err
	}
	if !reached.Fits() {
		return &ErrRegistrationRefused{Wait: reached.Wait}
	}
	return nil
}
