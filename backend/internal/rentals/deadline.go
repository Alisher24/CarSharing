package rentals

import (
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

// The deadline rules of a reservation. Each one is a pure function of a rental and a moment, so the
// sweep, the commands, the reads and the later ride commands decide alike without a clock of their
// own, and each reads the stage itself: a rental that is not a reservation answers no to all three.
//
// The moment a caller passes is the one its transaction read from the database after its locks
// rather than the moment a request arrived, which is what makes a wait for a lock unable to put a
// decision on the wrong side of a boundary.

// Reserved reports whether the rental still holds its vehicle as a reservation.
func (r Rental) Reserved() bool { return r.Stage == stage.Reserved }

// Overdue reports whether the reservation's deadline has been reached at an instant. The boundary
// itself counts: a reservation is due at exactly its deadline, not a moment later. It is the one
// precondition for leaving the reserved stage, whether the sweep, a command or a read meets it.
func (r Rental) Overdue(at time.Time) bool {
	return r.Reserved() && !at.Before(r.ExpiresAt)
}

// WarningDue reports whether the reservation has entered the minute before its deadline at an
// instant. The window is half-open, [expires_at - WarningLead, expires_at): at the deadline itself
// the reservation is overdue instead, so a warning is never created after the fact, and the
// comparison is on the stored moments without rounding them to seconds.
func (r Rental) WarningDue(at time.Time) bool {
	return r.Reserved() && !at.Before(r.ExpiresAt.Add(-WarningLead)) && at.Before(r.ExpiresAt)
}
