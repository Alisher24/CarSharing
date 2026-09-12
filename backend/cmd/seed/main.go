// Command seed inserts demo data and refuses to run outside the demo environment.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
)

// seedMarker is the row this command writes. It names the seed set, not a run, so a second run
// leaves the first one's date alone.
const seedMarker = "bootstrap-v1"

// errDemoEnvironmentRequired refuses a seed run outside the demo environment, where the demo data
// would land in a deployment that never asked for it.
var errDemoEnvironmentRequired = errors.New("seed requires APP_ENV=demo")

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
	if cfg.Environment != config.DemoEnvironment {
		return errDemoEnvironmentRequired
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.DatabaseStartupTimeout)
	defer cancel()
	pool, err := database.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `INSERT INTO seed_runs (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`,
		seedMarker)
	if err != nil {
		return fmt.Errorf("seed failed; run migrations first: %w", err)
	}

	slog.Info("bootstrap seed completed; demo users and vehicles are not implemented yet")
	return nil
}
