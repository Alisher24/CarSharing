package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
)

func main() {
	if !run() {
		os.Exit(1)
	}
}

func run() bool {
	if os.Getenv("APP_ENV") != "demo" {
		slog.Error("seed requires APP_ENV=demo")
		return false
	}
	c, err := config.Load()
	if err != nil {
		slog.Error(err.Error())
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, c)
	if err != nil {
		slog.Error(err.Error())
		return false
	}
	defer pool.Close()
	// This marker establishes repeatable seed execution. Users/fleet arrive with their schemas.
	_, err = pool.Exec(ctx, `INSERT INTO seed_runs (name) VALUES ('bootstrap-v1') ON CONFLICT (name) DO NOTHING`)
	if err != nil {
		slog.Error("seed failed; run migrations first")
		return false
	}
	slog.Info("bootstrap seed completed; demo users and vehicles are not implemented yet")
	return true
}
