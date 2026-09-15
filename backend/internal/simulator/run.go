package simulator

import (
	"context"
	"log/slog"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/periodic"
)

// TickInterval is how often the fleet is advanced. The model's result follows from the time that
// passed rather than from the number of calls, so an interval that drifts or a process that was
// stopped for a while costs nothing: the next tick advances the fleet by the whole of the time since
// the last one.
const TickInterval = time.Second

// RequestTimeout bounds one call of the tick operation. It is well inside the interval, so a call
// that hangs is given up on and the next one is made rather than the process waiting for ever.
const RequestTimeout = 5 * time.Second

// Run advances the fleet every TickInterval until the context is cancelled. It is the whole of what
// this process does: the model and its storage belong to the API, and this process is the clock that
// asks for the next moment.
func Run(ctx context.Context, client *Client) {
	periodic.Run(ctx, "simulation tick", TickInterval, func(ctx context.Context) (int64, error) {
		called, cancel := context.WithTimeout(ctx, RequestTimeout)
		defer cancel()

		result, err := client.Tick(called)
		if err != nil {
			return 0, err
		}
		return int64(len(result.ChangedVehicles) + len(result.CompletedRentals)), nil
	})
}

// TickOnce advances the fleet under one identifier and reports what it changed, which is how a
// demonstration drives the fleet by hand instead of by the clock. A call repeated with the same
// identifier reproduces the first answer, so a tick whose answer was lost costs no second movement.
func TickOnce(ctx context.Context, logger *slog.Logger, client *Client, tickID string) error {
	called, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	var (
		result TickResult
		err    error
	)
	if tickID == "" {
		result, err = client.Tick(called)
	} else {
		result, err = client.TickWith(called, tickID)
	}
	if err != nil {
		return err
	}
	logger.Info("tick applied",
		"tick_id", result.TickID,
		"processed_at", result.ProcessedAt,
		"changed_vehicles", len(result.ChangedVehicles),
		"completed_rentals", len(result.CompletedRentals))
	return nil
}
