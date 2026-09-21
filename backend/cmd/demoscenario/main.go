// Command demoscenario puts the prepared demonstration vehicles and their rentals back to the
// state the demonstration starts from. It is deliberately separate from the seed: seeding creates
// what is missing, and only this command returns what already exists.
//
// It refuses outright, changing nothing, when a person has rented one of the prepared vehicles.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/demo"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
	"github.com/jackc/pgx/v5/pgxpool"
)

// restoreTimeout bounds the whole command, so a restoration waiting on a lock another transaction
// holds gives up rather than blocking indefinitely.
const restoreTimeout = time.Minute

// errDemoEnvironmentRequired refuses a restoration outside the demo environment, where the
// prepared scenario does not belong.
var errDemoEnvironmentRequired = errors.New("scenario restore requires APP_ENV=demo")

func main() {
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
	if cfg.Environment.Name != config.DemoEnvironment {
		return errDemoEnvironmentRequired
	}

	ctx, cancel := context.WithTimeout(context.Background(), restoreTimeout)
	defer cancel()

	startup, cancelStartup := context.WithTimeout(ctx, database.DatabaseStartupTimeout)
	pool, err := database.Open(startup, cfg.Database)
	cancelStartup()
	if err != nil {
		return err
	}
	defer pool.Close()

	return restore(ctx, pool)
}

func restore(ctx context.Context, pool *pgxpool.Pool) error {
	stores, err := demo.NewRestoreStores(
		auth.NewUserStore(pool),
		fleet.NewStore(pool),
		rentals.NewStore(pool),
		simulation.NewStore(pool),
	)
	if err != nil {
		return err
	}
	if err = demo.Restore(ctx, pool, stores); err != nil {
		return err
	}

	slog.Info("demonstration scenario restored")
	return nil
}
