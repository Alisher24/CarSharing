package rentals

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// publishVehicleChange marks the vehicle as changed for every reader of the catalog and returns the
// version it reached. A version is raised rather than set, because a client compares the version of
// two readings to decide which one is newer.
//
// A ride that ran out of energy takes its vehicle out of service in the same statement, so the change
// a reader hears about and the reason it is unavailable are published together: a version raised
// without the flag would show a vehicle that is free to book until the next reading of it.
const publishVehicleChangeStatement = `
UPDATE vehicles
SET version = version + 1,
    service_required = service_required OR $2
WHERE id = $1
RETURNING version`

func publishVehicleChange(
	ctx context.Context, pool *pgxpool.Pool, vehicleID string, serviceRequired bool,
) (int64, error) {
	var version int64
	err := database.QuerierFrom(ctx, pool).QueryRow(ctx, publishVehicleChangeStatement,
		vehicleID, serviceRequired).Scan(&version)
	return version, err
}
