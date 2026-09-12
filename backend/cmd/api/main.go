// Command api serves the public HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpapi"
)

const (
	// databaseStartupTimeout bounds the wait for the database to accept connections. Compose
	// orders startup, so this only has to cover the first readiness of a cold container.
	databaseStartupTimeout = 45 * time.Second

	// shutdownTimeout is how long in-flight requests are given to finish after a signal.
	shutdownTimeout = 10 * time.Second
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
	startup, cancel := context.WithTimeout(ctx, databaseStartupTimeout)
	pool, err := database.Open(startup, cfg)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	server := &http.Server{
		Addr: cfg.HTTPAddr, Handler: httpapi.Router(httpapi.DatabaseProbe(pool)),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 16 << 10,
	}
	serverExit := make(chan error, 1)
	go func() { serverExit <- server.ListenAndServe() }()
	slog.Info("API started", "address", cfg.HTTPAddr)
	select {
	case err = <-serverExit:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server failed")
		}
	case <-ctx.Done():
		shutdown, done := context.WithTimeout(context.Background(), shutdownTimeout)
		defer done()
		if err = server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return errors.New("HTTP shutdown deadline exceeded")
		}
	}
	slog.Info("API stopped")
	return nil
}
