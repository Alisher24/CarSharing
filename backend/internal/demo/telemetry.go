package demo

import (
	"context"
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConfirmationInterval is how often the demonstration source confirms where the fleet is standing.
// It is comfortably inside fleet.MaxTelemetryAge, so an ordinary vehicle stays fresh between two
// confirmations and a vehicle the source skips goes stale on the ordinary rule instead.
const ConfirmationInterval = fleet.MaxTelemetryAge / 3

// Confirmations is the demonstration source of telemetry. It stands in for the vehicles of a
// demonstration that do not exist outside it, and it publishes what the model of each vehicle holds:
// where it stands and what is left in its sources. A vehicle the source does not confirm keeps the
// reading it last confirmed, which is what lets one demonstration show a position that has aged out
// of freshness rather than a position that was quietly replaced by a prediction.
//
// Reading the catalog is not a confirmation. Only an arrival recorded here makes a position fresh.
type Confirmations struct {
	pool     *pgxpool.Pool
	vehicles *fleet.Store
	models   *simulation.Store
}

// NewConfirmations assembles the demonstration telemetry source from its required record owners.
func NewConfirmations(
	pool *pgxpool.Pool,
	vehicles *fleet.Store,
	models *simulation.Store,
) (*Confirmations, error) {
	for _, dependency := range []struct {
		name     string
		supplied bool
	}{
		{"database pool", pool != nil},
		{"vehicle records", vehicles != nil},
		{"simulation models", models != nil},
	} {
		if !dependency.supplied {
			return nil, fmt.Errorf("demonstration confirmation dependency %s is missing", dependency.name)
		}
	}
	return &Confirmations{pool: pool, vehicles: vehicles, models: models}, nil
}

// Confirm records one arrival from every vehicle that is still reporting and returns how many
// vehicles confirmed. The signals of the new versions are recorded with the confirmation itself: a
// reading that was stored without telling anybody would leave every map showing an older one.
func (c *Confirmations) Confirm(ctx context.Context) (int64, error) {
	var confirmed int64
	err := database.InTransactionWithHandle(ctx, c.pool, func(txCtx context.Context, tx pgx.Tx) error {
		reporting, err := c.vehicles.ReportingVehicleIDs(txCtx, tx)
		if err != nil {
			return err
		}
		if len(reporting) == 0 {
			return nil
		}
		if err = c.publishArrivals(txCtx, tx, reporting); err != nil {
			return err
		}
		signals, err := c.confirmationSignals(txCtx, tx, reporting)
		if err != nil {
			return err
		}
		confirmed = int64(len(signals))
		return events.Record(txCtx, c.pool, signals...)
	})
	return confirmed, err
}

type confirmedArrivals struct {
	confirmations []fleet.Confirmation
	unmodelled    []string
}

// publishArrivals writes the confirmed reading of every reporting vehicle: where the model stands and
// what it holds for the vehicles it travels, and the moment alone for the vehicles it does not.
func (c *Confirmations) publishArrivals(
	ctx context.Context,
	tx pgx.Tx,
	reporting []string,
) error {
	states, err := c.models.States(ctx, reporting)
	if err != nil {
		return err
	}
	arrivals := arrivalsOf(reporting, states)
	for _, confirmation := range arrivals.confirmations {
		if err = c.vehicles.Confirm(ctx, tx, confirmation); err != nil {
			return err
		}
	}
	return c.vehicles.RefreshTelemetry(ctx, tx, arrivals.unmodelled)
}

// arrivalsOf states what one confirmation publishes about the reporting fleet.
func arrivalsOf(reporting []string, states map[string]simulation.State) confirmedArrivals {
	var arrivals confirmedArrivals
	for _, vehicleID := range reporting {
		state, modelled := states[vehicleID]
		if !modelled {
			// A vehicle nothing is modelled for still reports that it is where it was: the moment of
			// its reading moves even though its position and its reserves do not.
			arrivals.unmodelled = append(arrivals.unmodelled, vehicleID)
			continue
		}
		confirmation := fleet.Confirmation{
			VehicleID: vehicleID,
			Position:  state.Position,
		}
		for _, source := range state.Sources {
			confirmation.Sources = append(confirmation.Sources, fleet.EnergySource{
				Kind:      source.Kind,
				Remaining: source.Remaining(),
			})
		}
		arrivals.confirmations = append(arrivals.confirmations, confirmation)
	}
	return arrivals
}

// confirmationSignals raises the version of every confirmed vehicle and states the public change each
// one is.
func (c *Confirmations) confirmationSignals(
	ctx context.Context,
	tx pgx.Tx,
	reporting []string,
) ([]events.Signal, error) {
	signals := make([]events.Signal, 0, len(reporting))
	for _, vehicleID := range reporting {
		version, err := c.vehicles.PublishChange(ctx, tx, vehicleID, fleet.VehicleChange{})
		if err != nil {
			return nil, err
		}
		signals = append(signals, events.Signal{
			Kind:       events.VehicleChanged,
			ResourceID: vehicleID,
			Version:    version,
		})
	}
	return signals, nil
}
