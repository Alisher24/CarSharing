// Command api serves the public HTTP API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpserver"
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

	hub := events.NewHub(pool)
	serving, err := assemble(cfg, pool, hub)
	if err != nil {
		return err
	}
	startBackgroundWork(ctx, cfg, hub, serving.confirmations)
	return httpserver.Serve(ctx, httpserver.New(cfg.HTTPAddr, serving.handler))
}
