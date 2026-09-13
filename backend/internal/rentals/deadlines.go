package rentals

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Deadlines is the recurring pass over the reservations of the fleet: it releases the ones whose
// deadline has passed and creates the warning of those that have entered their last minute. Both are
// one pass because they are one question — what the deadline of a reservation means now — and they
// run in that order, so a reservation that has run out is released rather than warned about the
// minute it no longer has.
type Deadlines struct {
	expiry  *expiry
	warning *warning
}

func NewDeadlines(pool *pgxpool.Pool) *Deadlines {
	return &Deadlines{expiry: newExpiry(pool), warning: newWarning(pool)}
}

// Due runs one pass and reports how many reservations it moved: the ones it released and the ones it
// warned. A pass that fails reports what it had already done, so a worker that stops seeing the
// database keeps the count of the transitions that were committed before the failure.
func (d *Deadlines) Due(ctx context.Context) (int64, error) {
	ended, err := d.expiry.expireDue(ctx)
	if err != nil {
		return ended, err
	}
	warned, err := d.warning.warnDue(ctx)
	return ended + warned, err
}
