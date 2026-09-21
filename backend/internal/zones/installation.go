package zones

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/jackc/pgx/v5"
)

const insertZoneStatement = `
INSERT INTO service_zones (id, name, area, version)
VALUES ($1, $2, ST_SetSRID(ST_GeomFromGeoJSON($3), $4), $5)
ON CONFLICT (id) DO NOTHING`

// Install creates a service area that is absent and leaves an existing area unchanged.
func (s *Store) Install(ctx context.Context, tx pgx.Tx, zone Zone) error {
	_, err := tx.Exec(
		ctx,
		insertZoneStatement,
		zone.ID,
		zone.Name,
		string(zone.Area),
		fleet.WGS84SRID,
		zone.Version,
	)
	return err
}
