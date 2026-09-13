package rentals

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// raiseVehicleVersion marks the vehicle as changed for every reader of the catalog and returns the
// version it reached. A version is raised rather than set, because a client compares the version of
// two readings to decide which one is newer.
const raiseVehicleVersionStatement = `
UPDATE vehicles SET version = version + 1 WHERE id = $1 RETURNING version`

func raiseVehicleVersion(ctx context.Context, pool *pgxpool.Pool, vehicleID string) (int64, error) {
	var version int64
	err := database.QuerierFrom(ctx, pool).QueryRow(ctx, raiseVehicleVersionStatement, vehicleID).
		Scan(&version)
	return version, err
}
