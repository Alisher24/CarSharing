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
	if os.Getenv("APP_ENV") != "demo" {
		return errDemoEnvironmentRequired
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.DatabaseStartupTimeout)
	defer cancel()
	pool, err := database.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `INSERT INTO seed_runs (name) VALUES ('bootstrap-v1') ON CONFLICT (name) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("seed failed; run migrations first: %w", err)
	}

	slog.Info("bootstrap seed completed; demo users and vehicles are not implemented yet")
	return nil
}
