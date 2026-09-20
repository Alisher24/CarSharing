// Command migrate applies the database migrations or reports their status.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/Alisher24/CarSharing/backend/db/migrations"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// migrationTimeout bounds the whole command, so a lock held by another process ends the run rather
// than waiting for it forever.
const migrationTimeout = 2 * time.Minute

// Reasons this command refuses to run. Each names the next step an operator should take, because a
// migration failure is answered by a person rather than by a retry.
var (
	errUsage           = errors.New("usage: migrate [up|status]")
	errPoolConfig      = errors.New("invalid database configuration")
	errMigrationLock   = errors.New("cannot configure migration lock")
	errMigrationFailed = errors.New("migration failed; check database availability, role and migration SQL")
)

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	command, err := migrationCommand(os.Args[1:])
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	poolConfig, err := database.PoolConfig(cfg.Database)
	if err != nil {
		return errPoolConfig
	}

	db := stdlib.OpenDB(*poolConfig.ConnConfig)
	defer db.Close()
	return migrate(db, command)
}

// migrationCommand reads the one command this process accepts, defaulting to applying the
// migrations. An argument list of any other shape is a usage error rather than a command to guess at.
func migrationCommand(arguments []string) (string, error) {
	command := "up"
	if len(arguments) == 1 {
		command = arguments[0]
	}
	if len(arguments) > 1 || command != "up" && command != "status" {
		return "", errUsage
	}
	return command, nil
}

func migrate(db *sql.DB, command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), migrationTimeout)
	defer cancel()

	// The migration set is opened under a PostgreSQL session lock, so two processes starting at
	// once apply the migrations one at a time rather than racing each other.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return errMigrationLock
	}
	provider, err := goose.NewProvider(
		goose.DialectPostgres, db, migrations.Files, goose.WithSessionLocker(locker),
	)
	if err != nil {
		return errMigrationFailed
	}

	if command == "status" {
		if err = reportStatus(ctx, provider); err != nil {
			return errMigrationFailed
		}
	} else if _, err = provider.Up(ctx); err != nil {
		return errMigrationFailed
	}

	slog.Info("migration command completed", "command", command)
	return nil
}

// reportStatus logs each migration and the state it is in, so an operator can see which ones are
// still pending without applying them.
func reportStatus(ctx context.Context, provider *goose.Provider) error {
	statuses, err := provider.Status(ctx)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		slog.Info("migration", "version", status.Source.Version, "state", status.State)
	}
	return nil
}
