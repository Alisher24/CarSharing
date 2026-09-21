package fleet

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// VehicleChange is what one stored change publishes about a vehicle. An absent field keeps its
// current value; a present field replaces it. Every change advances the public version independently
// of which fields it carries.
type VehicleChange struct {
	Connected       *bool
	Reporting       *bool
	RouteID         *string
	ServiceRequired *bool
}

// publishChangeStatement is the one rule for changing a vehicle: its version is raised rather than
// supplied by a caller, so no path can move the public sequence backwards.
const publishChangeStatement = `
UPDATE vehicles
SET version = version + 1,
    connected = coalesce($2::boolean, connected),
    reporting = coalesce($3::boolean, reporting),
    route_id = coalesce($4::text, route_id),
    service_required = coalesce($5::boolean, service_required)
WHERE id = $1
RETURNING version`

// PublishChange stores one vehicle change in the transaction that owns the surrounding operation
// and returns the version the change reached.
func (s *Store) PublishChange(
	ctx context.Context,
	tx pgx.Tx,
	vehicleID string,
	change VehicleChange,
) (int64, error) {
	var version int64
	err := tx.QueryRow(
		ctx,
		publishChangeStatement,
		vehicleID,
		change.Connected,
		change.Reporting,
		change.RouteID,
		change.ServiceRequired,
	).Scan(&version)
	return version, err
}
