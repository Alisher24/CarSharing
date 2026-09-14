package rentals

import (
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

// The four boundaries of a reservation made at 10:00:00, to the microsecond: the warning is due from
// 10:14:00 and the reservation is overdue from 10:15:00, and neither rule reaches into the other's
// moment. The moments before and after each boundary state which side of it the rules change on.
func TestDeadlineBoundaries(t *testing.T) {
	reservedAt := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC)
	reservation := reservationAt(reservedAt)

	for _, boundary := range []struct {
		name    string
		moment  time.Time
		warning bool
		overdue bool
	}{
		{"10:13:59.999999", reservedAt.Add(14*time.Minute - time.Microsecond), false, false},
		{"10:14:00.000000", reservedAt.Add(14 * time.Minute), true, false},
		{"10:14:59.999999", reservedAt.Add(15*time.Minute - time.Microsecond), true, false},
		{"10:15:00.000000", reservedAt.Add(15 * time.Minute), false, true},
		{"10:15:00.000001", reservedAt.Add(15*time.Minute + time.Microsecond), false, true},
	} {
		at := boundary.moment
		if got := reservation.WarningDue(at); got != boundary.warning {
			t.Errorf("at %s a warning is due %t, want %t", boundary.name, got, boundary.warning)
		}
		if got := reservation.Overdue(at); got != boundary.overdue {
			t.Errorf("at %s the reservation is overdue %t, want %t", boundary.name, got, boundary.overdue)
		}
	}
}

// A rental that is not a reservation is never warned and never overdue, whatever the moment: a ride,
// a cancelled reservation and an expired one are history rather than a deadline.
func TestDeadlineRulesIgnoreRentalsThatAreNotReserved(t *testing.T) {
	reservedAt := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC)
	for _, notReserved := range []stage.Stage{
		stage.Active,
		stage.Paused,
		stage.Cancelled,
		stage.Expired,
		stage.Completed,
	} {
		rental := reservationAt(reservedAt)
		rental.Stage = notReserved
		for _, at := range []time.Time{
			reservedAt,
			reservedAt.Add(14 * time.Minute),
			reservedAt.Add(15 * time.Minute),
			reservedAt.Add(time.Hour),
		} {
			if rental.WarningDue(at) {
				t.Errorf("a %s rental is warned at %s", notReserved, at)
			}
			if rental.Overdue(at) {
				t.Errorf("a %s rental is overdue at %s", notReserved, at)
			}
		}
	}
}

// A rental that was never read — the zero value, which is what a caller without a rental holds —
// answers no to every rule rather than treating its zero deadline as one that has passed.
func TestDeadlineRulesIgnoreAnAbsentRental(t *testing.T) {
	at := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC)
	var absent Rental
	if absent.Reserved() {
		t.Error("an absent rental is reserved")
	}
	if absent.WarningDue(at) {
		t.Error("an absent rental is warned")
	}
	if absent.Overdue(at) {
		t.Error("an absent rental is overdue")
	}
}

// The warning window is the minute the lifetime leaves: it opens one lead before the deadline and
// closes at it, so a reservation outside that minute is never warned.
func TestWarningWindowFollowsTheDeclaredLead(t *testing.T) {
	reservedAt := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC)
	reservation := reservationAt(reservedAt)

	if got, want := reservation.ExpiresAt.Sub(reservedAt), ReservationLifetime; got != want {
		t.Fatalf("the deadline is %s after the reservation, want %s", got, want)
	}
	opensAt := reservation.ExpiresAt.Add(-WarningLead)
	if reservation.WarningDue(opensAt.Add(-time.Microsecond)) {
		t.Error("a warning is due before the window opens")
	}
	if !reservation.WarningDue(opensAt) {
		t.Error("a warning is not due when the window opens")
	}
}

// reservationAt is one reservation as a reader of the rules sees it: the stage and the two moments
// the deadline is judged by.
func reservationAt(reservedAt time.Time) Rental {
	return Rental{
		Stage:      stage.Reserved,
		ReservedAt: reservedAt,
		ExpiresAt:  reservedAt.Add(ReservationLifetime),
	}
}
