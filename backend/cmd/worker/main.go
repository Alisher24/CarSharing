// Command worker runs the background work of the backend: the delivery of the tasks the outbox
// holds, the release of reservations whose deadline has passed, and the retention of the records
// that outlive the work they describe. It is a separate process from the API so that either one can
// be restarted without stopping the other, and both use the same modules rather than a second copy
// of the rules.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
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
