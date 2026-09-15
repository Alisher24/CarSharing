// Command simulator advances the modelled fleet: it calls the internal tick operation once a second
// and reports what the call changed. It is a process of its own so that restarting it leaves the API
// serving, and so that the fleet stands still when it is stopped rather than being advanced by
// whatever else happens to be running.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/simulator"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	once := flag.Bool("once", false, "advance the fleet once and exit")
	tickID := flag.String("tick-id", "", "the identifier of the one tick to advance the fleet under")
	flag.Parse()

	cfg, err := config.SimulatorClient()
	if err != nil {
		return err
	}
	client, err := simulator.NewClient(cfg.APIURL, cfg.Token)
	if err != nil {
		return err
	}
	if *once {
		return simulator.TickOnce(context.Background(), slog.Default(), client, *tickID)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("simulator started",
		"api", cfg.APIURL, "interval", simulator.TickInterval.String())
	simulator.Run(ctx, client)
	slog.Info("simulator stopped")
	return nil
}
