// Command seed installs the demonstration data and refuses to run outside the demo environment.
// It creates what is missing and changes nothing that is already there, so a second run and a
// restart both leave the demonstration exactly as it stands.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/demo"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/Alisher24/CarSharing/backend/internal/zones"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedMarker is the row this command writes. It names the seed set, not a run, so a second run
// leaves the first one's date alone.
const seedMarker = "bootstrap-v1"

// seedTimeout bounds the whole installation, which hashes a password for every demonstration
// account before it writes anything.
const seedTimeout = 5 * time.Minute

// Reasons this command refuses to run. Each names what an operator must supply rather than letting
// the run continue with a value it invented.
var (
	errDemoEnvironmentRequired = errors.New("seed requires APP_ENV=demo")
	errDemoPasswordRequired    = errors.New("seed requires " + config.DemoUserPasswordFileVariable)
)

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
	if cfg.DemoUserPassword == "" {
		return errDemoPasswordRequired
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()

	startup, cancelStartup := context.WithTimeout(ctx, database.DatabaseStartupTimeout)
	pool, err := database.Open(startup, cfg.Database)
	cancelStartup()
	if err != nil {
		return err
	}
	defer pool.Close()

	return install(ctx, pool, cfg)
}

func install(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) error {
	if err := recordSeedSet(ctx, pool); err != nil {
		return err
	}
	stores, err := demo.NewSeedStores(
		auth.NewUserStore(pool),
		fleet.NewStore(pool),
		rentals.NewStore(pool),
		tariffs.NewStore(pool),
		zones.NewStore(pool),
	)
	if err != nil {
		return err
	}
	if err = demo.Seed(
		ctx,
		pool,
		stores,
		auth.NewPasswordHasher(cfg.Argon2),
		cfg.DemoUserPassword,
	); err != nil {
		return err
	}

	slog.Info("demonstration seed completed", "vehicles", len(demo.Fleet()))
	return nil
}

func recordSeedSet(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO seed_runs (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, seedMarker)
	if err != nil {
		return fmt.Errorf("seed failed; run migrations first: %w", err)
	}
	return nil
}
