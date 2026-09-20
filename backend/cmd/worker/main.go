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

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
	"github.com/Alisher24/CarSharing/backend/internal/outbox"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/platform/periodic"
	"github.com/Alisher24/CarSharing/backend/internal/platform/ratelimit"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
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
	pool, err := database.Open(startup, cfg.Database)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	// The address and the credential of the mail stub are read here, where the process is assembled,
	// and handed to the delivery: nothing below this line reads the environment.
	letters, err := config.MailstubClient()
	if err != nil {
		return err
	}
	return work(ctx, pool, letters)
}

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
	go delivery.Run(ctx)
	go periodic.Run(ctx, "reservation deadlines", rentals.DeadlineSweepInterval,
		rentals.NewDeadlines(pool).Due)
	go periodic.Run(ctx, "signal retention", events.RetentionInterval, events.NewReaper(pool).Delete)
	go periodic.Run(ctx, "command result retention", idempotency.RetentionInterval,
		idempotency.NewReaper(pool).Delete)
	go periodic.Run(ctx, "sign-in counter retention", ratelimit.RetentionInterval,
		ratelimit.NewReaper(pool).Delete)
	go periodic.Run(ctx, "session retention", sessions.RetentionInterval, sessions.NewReaper(pool).Delete)

	slog.Info("worker started")
	<-ctx.Done()
	slog.Info("worker stopped")
	return nil
}

// deliveries is the table of what this process delivers, assembled where the process is: the signals
// the queue carries to every API process, the first attempt at the payment of an invoice, and the
// letter that carries the invoice to the account that owes it.
//
// A task of a kind this table does not name is kept in the queue with its error, so a kind whose
// delivery belongs to a later task is owed rather than reported as delivered.
func deliveries(pool *pgxpool.Pool, letters config.InternalClient) (outbox.Deliveries, error) {
	payments, err := rentals.NewRentalPayment(pool)
	if err != nil {
		return nil, err
	}
	posted, err := mailstub.NewClient(letters.APIURL, letters.Token)
	if err != nil {
		return nil, err
	}
	delivery, err := mailstub.NewDelivery(invoices.NewStore(pool), auth.NewUserStore(pool), posted)
	if err != nil {
		return nil, err
	}
	table := events.Deliveries(pool)
	table[rentals.PaymentAttemptTask()] = func(ctx context.Context, task outbox.Task) error {
		_, err := payments.Attempt(ctx, task.ResourceID)
		return err
	}
	table[rentals.InvoiceIssuedTask()] = func(ctx context.Context, task outbox.Task) error {
		return delivery.Deliver(ctx, task.ResourceID)
	}
	return table, nil
}
