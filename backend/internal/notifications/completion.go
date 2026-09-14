package notifications

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Completion is what the report of a finished ride tells about beyond the ride itself: the invoice the
// ride produced and why it ended. A warning about a reservation states neither, so the record states
// one or the other rather than a half-filled pair.
type Completion struct {
	InvoiceID string
	Reason    completion.Reason
}

// Completer writes the one notification that reports a finished ride. It is what the module that ends
// a ride needs and nothing more, so the two do not learn about each other's records: the statement it
// runs is the notification table's own, and the transaction it runs in is the one the ending opened.
type Completer struct{ pool *pgxpool.Pool }

// NewCompleter returns the writer of completion reports over one connection pool.
func NewCompleter(pool *pgxpool.Pool) *Completer { return &Completer{pool: pool} }

// insertCompletionStatement writes the report of a finished ride unless this rental already has one.
// The conflict is the rule rather than a failure: a repeated attempt writes nothing, so the moment and
// the invoice the first one stored are never replaced.
const insertCompletionStatement = `
INSERT INTO notifications (
    id, user_id, rental_id, kind, created_at, active, version, invoice_id, completion_reason
)
VALUES ($1, $2, $3, $4, $5, true, $6, $7, $8)
ON CONFLICT (rental_id, kind) DO NOTHING`

// Record stores the report of one finished ride at the moment the ending transaction fixed. The
// personal signal of the new notification is recorded with it, so the account that finished the ride
// hears about the report in the transaction that wrote it.
func (c *Completer) Record(
	ctx context.Context, owner uuid.UUID, rentalID string, about Completion, at time.Time,
) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	written, err := database.QuerierFrom(ctx, c.pool).Exec(ctx, insertCompletionStatement,
		id.String(), owner, rentalID, RentalCompleted, at, initialVersion, about.InvoiceID, about.Reason)
	if err != nil {
		return err
	}
	if written.RowsAffected() == 0 {
		// The report is already there from the transaction that ended the ride first, and a stored
		// report is never replaced: no second signal describes a change that did not happen.
		return nil
	}
	return NewStore(c.pool).announce(ctx, Notification{
		ID:       id.String(),
		UserID:   owner,
		RentalID: rentalID,
		Version:  initialVersion,
	})
}
