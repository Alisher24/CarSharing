package events

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

// How long a delivered signal task is kept. The policy covers the signals of this package only: a
// task of any other kind, and a task that has not been delivered, is not deleted by it.
const (
	// RetentionPeriod is how long a delivered signal task stays in the queue after it was delivered.
	// It is long enough to answer what was announced recently and short enough that the queue does
	// not grow with the history of a running demonstration.
	RetentionPeriod = 24 * time.Hour

	// RetentionBatchSize is how many tasks one sweep deletes, so a queue that has accumulated for a
	// long time is emptied over several sweeps instead of in one long transaction. A demonstration
	// fleet confirms its telemetry every few seconds, so the sweeps together have to carry more than
	// the tasks one day of running produces.
	RetentionBatchSize = 1000

	// RetentionInterval is how often a worker sweeps the queue. A sweep is an indexed delete over a
	// set that stays small, so running it often is what keeps each portion small.
	RetentionInterval = 10 * time.Second
)

// Reaper deletes delivered signal tasks once their retention has passed. It is one of the recurring
// jobs of the worker process.
type Reaper struct {
	store *outbox.Store
}

func NewReaper(pool *pgxpool.Pool) *Reaper { return &Reaper{store: outbox.NewStore(pool)} }

// Delete runs one sweep and reports how many tasks it removed.
func (r *Reaper) Delete(ctx context.Context) (int64, error) {
	return r.store.DeleteCompleted(ctx, AnnouncedKindNames(), RetentionPeriod, RetentionBatchSize)
}
