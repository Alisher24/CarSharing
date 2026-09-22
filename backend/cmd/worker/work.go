package main

import (
	"context"
	"log/slog"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/outbox"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/periodic"
	"github.com/Alisher24/CarSharing/backend/internal/platform/retention"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/jackc/pgx/v5/pgxpool"
)

// work runs the recurring jobs of this process until it is asked to stop. Each job is given the
// behaviour it performs and the schedule it runs on; the process only decides that they run at all.
//
// Two workers may run side by side: the queue hands a task to one attempt at a time under a lease
// that runs out on its own, the letter of an invoice is stored under a key the receiver deduplicates,
// and the deadline pass performs one transition per reservation — its release or its warning —
// whichever process performs it.
func work(ctx context.Context, pool *pgxpool.Pool, letters config.InternalClient) error {
	deliveries, err := deliveries(pool, letters)
	if err != nil {
		return err
	}
	delivery, err := outbox.NewWorker(pool, deliveries.Deliver)
	if err != nil {
		return err
	}
	notificationStore := notifications.NewStore(pool)
	deadlines, err := rentals.NewDeadlines(
		pool,
		fleet.NewStore(pool),
		rentals.WarningOperations{
			Create: notificationStore.CreateReservationWarning,
			End:    notificationStore.EndReservationWarning,
		},
	)
	if err != nil {
		return err
	}
	slog.Info("worker started")
	periodic.RunAll(ctx, delivery.Run,
		func(ctx context.Context) {
			periodic.Run(ctx, "reservation deadlines", rentals.DeadlineSweepInterval, deadlines.Due)
		},
		func(ctx context.Context) { retention.Run(ctx, pool, retentionSweeps()) },
	)

	slog.Info("worker stopped")
	return nil
}
