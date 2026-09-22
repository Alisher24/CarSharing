package rentals

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// announceRentalChange raises the version of the vehicle a rental holds and records the signals of both
// changes, so the catalog a visitor reads and the account that holds the rental hear about the change in
// the transaction that made it.
//
// A ride that moved and a rental that ended announce the same two changes and differ only in the rental
// each of them names, so the announcement is one function over the rental as the transition left it.
func announceRentalChange(
	ctx context.Context,
	pool *pgxpool.Pool,
	vehicles *fleet.Store,
	tx pgx.Tx,
	changed Rental,
) error {
	version, err := vehicles.PublishChange(ctx, tx, changed.VehicleID, fleet.VehicleChange{})
	if err != nil {
		return err
	}
	return events.Record(ctx, pool,
		events.Signal{Kind: events.RentalChanged, ResourceID: changed.ID,
			Version: changed.Version, Recipient: changed.UserID},
		events.Signal{Kind: events.VehicleChanged, ResourceID: changed.VehicleID, Version: version},
	)
}
