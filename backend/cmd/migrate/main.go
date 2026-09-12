package main

import (
	"context"
	"database/sql"
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

func main() {
	if !run() {
		os.Exit(1)
	}
}

func run() bool {
	command := "up"
	if len(os.Args) == 2 {
		command = os.Args[1]
	}
	if len(os.Args) > 2 || (command != "up" && command != "status") {
		slog.Error("usage: migrate [up|status]")
		return false
	}
	c, err := config.Load()
	if err != nil {
		slog.Error(err.Error())
		return false
	}
	pc, err := database.PoolConfig(c)
	if err != nil {
		slog.Error("invalid database configuration")
		return false
	}
	db := stdlib.OpenDB(*pc.ConnConfig)
	defer db.Close()
	return migrate(db, command)
}

func migrate(db *sql.DB, command string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		slog.Error("cannot configure migration lock")
		return false
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files, goose.WithSessionLocker(locker))
	if err == nil {
		if command == "status" {
			var status []*goose.MigrationStatus
			status, err = provider.Status(ctx)
			if err == nil {
				for _, s := range status {
					slog.Info("migration", "version", s.Source.Version, "state", s.State)
				}
			}
		} else {
			_, err = provider.Up(ctx)
		}
	}
	if err != nil {
		slog.Error("migration failed; check database availability, role and migration SQL")
		return false
	}
	slog.Info("migration command completed", "command", command)
	return true
}
