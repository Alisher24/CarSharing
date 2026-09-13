package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// What one attempt is given and what it is bounded by. A lease outlives the delivery it covers many
// times over, so an attempt that is still running when its delivery limit is reached is a failure of
// the delivery rather than a task that has to be taken away from its worker.
const (
	// leaseDuration is how long one claim owns a task before another worker may take it over. It is
	// long enough to cover a delivery and its retry bookkeeping, and short enough that the work of a
	// worker that died is picked up while somebody is still watching.
	leaseDuration = 30 * time.Second

	// deliveryTimeout bounds one delivery, so an unresponsive recipient fails the attempt instead of
	// holding the task until the lease runs out.
	deliveryTimeout = 5 * time.Second

	// pollInterval is how often a worker with nothing to do looks at the queue.
	pollInterval = 250 * time.Millisecond

	// problemAttempts is the attempt from which a task is reported as a problem rather than as an
	// ordinary retry. A task that reaches it is kept and keeps being retried.
	problemAttempts = 10

	// statusInterval is how often a worker reports what it did and what the queue still holds, so a
	// running process is visible even when it has nothing to deliver.
	statusInterval = 30 * time.Second
)

// ErrIncompleteWorker refuses to run a worker that was not given what it delivers with. A worker
// that started without it would claim tasks it can never settle.
var ErrIncompleteWorker = errors.New("the outbox worker is missing a dependency")

// Worker delivers the tasks the queue holds: it claims one task at a time under a short lease,
// delivers it with no lock held, and records the outcome under the token of that attempt.
type Worker struct {
	store   *Store
	deliver Deliver

	// delivered and failed count the attempts since the last report. Run is the only reader and
	// writer of both, so one worker's counters need no synchronisation.
	delivered int64
	failed    int64
}

// NewWorker builds the worker over the queue in PostgreSQL. A delivery is required rather than
// defaulted, because a worker that reached a nil one would claim tasks it could never deliver.
func NewWorker(pool *pgxpool.Pool, deliver Deliver) (*Worker, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: database pool", ErrIncompleteWorker)
	}
	if deliver == nil {
		return nil, fmt.Errorf("%w: delivery", ErrIncompleteWorker)
	}
	return &Worker{store: NewStore(pool), deliver: deliver}, nil
}

// Run delivers due tasks until ctx is done. Two workers may run at once: a task is claimed by one
// attempt, and a second worker either finds nothing due or takes over a lease that has run out.
func (w *Worker) Run(ctx context.Context) {
	poll := time.NewTicker(pollInterval)
	defer poll.Stop()
	status := time.NewTicker(statusInterval)
	defer status.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-status.C:
			w.report(ctx)
		case <-poll.C:
			w.deliverDue(ctx)
		}
	}
}

// deliverDue delivers everything that is due, so a worker that finds work keeps going rather than
// waiting for the next poll.
func (w *Worker) deliverDue(ctx context.Context) {
	for ctx.Err() == nil {
		found, err := w.deliverNext(ctx)
		if err != nil {
			slog.Error("outbox claim failed", "error", err.Error())
			return
		}
		if !found {
			return
		}
	}
}

// deliverNext claims one task and delivers it, reporting whether the queue had anything due.
func (w *Worker) deliverNext(ctx context.Context) (bool, error) {
	claim, claimed, err := w.store.Claim(ctx, leaseDuration)
	if err != nil || !claimed {
		return false, err
	}
	w.deliverClaim(ctx, claim)
	return true, nil
}

// deliverClaim runs one attempt. The delivery is bounded by its own deadline, and the outcome is
// recorded only while this attempt still owns the task: an attempt whose lease ran out leaves the
// task to whoever holds it now.
func (w *Worker) deliverClaim(ctx context.Context, claim Claim) {
	attempt, cancel := context.WithTimeout(ctx, deliveryTimeout)
	err := w.deliver(attempt, claim.Task)
	cancel()

	// A worker that is stopping settles nothing: the task keeps its lease and returns to the queue
	// when the lease runs out, which is the same recovery a killed worker gets.
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		w.recordFailure(ctx, claim, err)
		return
	}
	settled, settleErr := w.store.Complete(ctx, claim)
	switch {
	case settleErr != nil:
		slog.Error("outbox task could not be completed", "task", claim.Task.ID.String(),
			"kind", claim.Task.Kind, "error", settleErr.Error())
	case !settled:
		slog.Warn("outbox lease was taken over before the task was completed",
			"task", claim.Task.ID.String(), "kind", claim.Task.Kind)
	default:
		w.delivered++
	}
}

// recordFailure keeps the task, states when it may be tried again, and reports the attempt. The
// failure is written under the attempt's own token, so a worker that comes back after its lease
// expired cannot record an error against the attempt of the worker that took over.
func (w *Worker) recordFailure(ctx context.Context, claim Claim, failure error) {
	delay := retryDelay(claim.Task.Attempts)
	settled, err := w.store.Fail(ctx, claim, failure.Error(), delay)
	if err != nil {
		slog.Error("outbox failure could not be recorded", "task", claim.Task.ID.String(),
			"kind", claim.Task.Kind, "error", err.Error())
		return
	}
	if !settled {
		slog.Warn("outbox lease was taken over before the failure was recorded",
			"task", claim.Task.ID.String(), "kind", claim.Task.Kind)
		return
	}
	w.failed++
	w.logFailure(claim, failure, delay)
}

// logFailure reports one failed attempt. A task that has reached the problem threshold is logged as
// an error of its own, because it will keep being retried and somebody has to look at it.
func (w *Worker) logFailure(claim Claim, failure error, delay time.Duration) {
	attributes := []any{
		"task", claim.Task.ID.String(),
		"kind", claim.Task.Kind,
		"resource", claim.Task.ResourceID,
		"attempts", claim.Task.Attempts,
		"retry_in", delay.String(),
		"error", failure.Error(),
	}
	if claim.Task.Attempts >= problemAttempts {
		slog.Error("outbox task keeps failing", attributes...)
		return
	}
	slog.Warn("outbox delivery failed", attributes...)
}

// report states what the worker did since the previous report and what the queue still holds, so a
// worker that is running but idle is distinguishable from one that is not running at all.
func (w *Worker) report(ctx context.Context) {
	pending, err := w.store.Pending(ctx)
	if err != nil {
		slog.Error("outbox status could not be read", "error", err.Error())
		return
	}
	slog.Info("worker heartbeat",
		"delivered", w.delivered,
		"failed", w.failed,
		"pending", pending.Count,
		"oldest_pending", pending.OldestAge.String())
	w.delivered, w.failed = 0, 0
}
