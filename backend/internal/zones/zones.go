// Package zones reads the operator service areas the public map draws. A zone is stored in WGS84
// and published exactly as it is stored: nothing here invents a boundary for a zone that could not
// be read.
package zones

import (
	"context"
	"encoding/json"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Zone is one service area. Its boundary is part of it: a position on the edge is covered.
type Zone struct {
	ID      string
	Name    string
	Version int64

	// Area is the boundary as GeoJSON, in the coordinate order the contract publishes. It is
	// carried as the database rendered it rather than re-encoded, so no step rounds a coordinate.
	Area json.RawMessage
}

// Store reads the service areas currently in force.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// geoJSONDecimals is how many decimal places a published coordinate keeps. Seven places is about a
// centimetre, well inside what a service boundary distinguishes.
const geoJSONDecimals = 7

const currentZones = `
SELECT id, name, version, ST_AsGeoJSON(area, $1)
FROM service_zones
ORDER BY id`

// Current reads every service area in force, in a stable order.
func (s *Store) Current(ctx context.Context) ([]Zone, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, currentZones, geoJSONDecimals)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := []Zone{}
	for rows.Next() {
		var zone Zone
		if err = rows.Scan(&zone.ID, &zone.Name, &zone.Version, &zone.Area); err != nil {
			return nil, err
		}
		found = append(found, zone)
	}
	return found, rows.Err()
}
