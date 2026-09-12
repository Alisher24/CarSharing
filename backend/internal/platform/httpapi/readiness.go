package httpapi

import (
	"context"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadinessProbe reports whether the dependencies this deployment needs are usable, along with the
// metadata they carry. The owning application supplies it, so the HTTP layer stays independent of
// the store behind it and the tests can answer without a database.
type ReadinessProbe func(context.Context) (servedapi.ReadyStatus, error)

// DatabaseProbe reads the bootstrap metadata and the PostGIS version in one round trip. The
// version itself is discarded: selecting it proves the extension is installed, which together with
// the returned row proves the connection and the migrated schema.
func DatabaseProbe(pool *pgxpool.Pool) ReadinessProbe {
	return func(ctx context.Context) (servedapi.ReadyStatus, error) {
		var readiness servedapi.ReadyStatus
		var postgisVersion string
		err := pool.QueryRow(ctx, `SELECT city, currency, timezone, postgis_version()
			FROM bootstrap_metadata WHERE singleton = true`).Scan(
			&readiness.City, &readiness.Currency, &readiness.Timezone, &postgisVersion)
		return readiness, err
	}
}
