// Package periodic runs recurring background work for as long as a process lives. It owns the
// schedule and the failure logging only: what the work is, and how often it should happen, are the
// caller's to decide.
package periodic

import (
	"context"
	"log/slog"
	"time"
)

// Work is one run of a recurring task. It reports how many records it touched, so a run that did
// nothing is not logged as if it had.
type Work func(context.Context) (int64, error)

// Run calls work every interval until ctx is done. A failed run is logged and the schedule keeps
// its cadence, because one unreachable database must not stop every later run.
func Run(ctx context.Context, name string, interval time.Duration, work Work) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce(ctx, name, work)
		}
	}
}

func runOnce(ctx context.Context, name string, work Work) {
	touched, err := work(ctx)
	if err != nil {
		slog.Error("background work failed", "work", name, "error", err.Error())
		return
	}
	if touched > 0 {
		slog.Info("background work completed", "work", name, "records", touched)
	}
}
