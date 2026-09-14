package rentals

import "time"

// ReservationLifetime is how long a reservation stands before it expires.
const ReservationLifetime = 15 * time.Minute

// WarningLead is how long before the deadline the reservation warns its holder. It is declared beside
// the lifetime because the two together state the deadline: the warning window is the half-open
// [expires_at - WarningLead, expires_at), and every other place derives its moments from them.
const WarningLead = time.Minute
