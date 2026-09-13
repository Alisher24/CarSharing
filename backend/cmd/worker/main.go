// Command worker runs the background work of the backend: the delivery of the tasks the outbox
// holds, the release of reservations whose deadline has passed, and the retention of the records
// that outlive the work they describe. It is a separate process from the API so that either one can be restarted without
// stopping the other, and both use the same modules rather than a second copy of the rules.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/outbox"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/platform/periodic"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startup, cancel := context.WithTimeout(ctx, database.DatabaseStartupTimeout)
	pool, err := database.Open(startup, cfg)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	return work(ctx, pool)
}

// work runs the recurring jobs of this process until it is asked to stop. Each job is given the
// behaviour it performs and the schedule it runs on; the process only decides that they run at all.
//
// Two workers may run side by side: the queue hands a task to one attempt at a time under a lease
// that runs out on its own, and the reservation sweep performs one transition per reservation
// whichever process performs it.
func work(ctx context.Context, pool *pgxpool.Pool) error {
	delivery, err := outbox.NewWorker(pool, events.Deliveries(pool).Deliver)
	if err != nil {
		return err
	}
	go delivery.Run(ctx)
	go periodic.Run(ctx, "reservation expiry", rentals.ExpirySweepInterval, rentals.NewExpiry(pool).ExpireDue)
	go periodic.Run(ctx, "signal retention", events.RetentionInterval, events.NewReaper(pool).Delete)
	go periodic.Run(ctx, "command result retention", idempotency.RetentionInterval,
		idempotency.NewReaper(pool).Delete)

	slog.Info("worker started")
	<-ctx.Done()
	slog.Info("worker stopped")
	return nil
}
