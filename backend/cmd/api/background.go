package main

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/demo"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/periodic"
)

// startBackgroundWork starts the recurring work this process owns. Listening for published signals
// is how a stream learns that something changed, so it always runs. The demonstration telemetry
// source stands in for vehicles that do not exist outside a demonstration, so it runs only where the
// demonstration does. Releasing reservations that have run out belongs to the worker process, which
// is the one place that decides a deadline has passed.
func startBackgroundWork(
	ctx context.Context, cfg config.Config, hub *events.Hub, confirmations *demo.Confirmations,
) {
	go hub.Run(ctx)
	if cfg.Environment.Name != config.DemoEnvironment {
		return
	}
	go periodic.Run(ctx, "demonstration telemetry", demo.ConfirmationInterval, confirmations.Confirm)
}
