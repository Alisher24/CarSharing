package httpapi

import (
	"context"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadyPath is the path of the readiness operation. It is exported because the container's own
// health check calls the same operation and must not carry a second copy of the path.
const ReadyPath = "/api/v1/health/ready"

// ReadinessProbe reports whether the dependencies this deployment needs are usable, along with the
// metadata they carry. The owning application supplies it, so the HTTP layer stays independent of
// the store behind it and the tests can answer without a database.
type ReadinessProbe func(context.Context) (servedapi.ReadyStatus, error)

// ReadyMetadata is the installation declaration readiness publishes.
type ReadyMetadata struct {
	City     string
	Currency string
	Timezone string
}

// ReadReadyMetadata is the record-owner operation supplied to the readiness probe.
type ReadReadyMetadata func(context.Context) (ReadyMetadata, error)

const postGISVersionSelection = `SELECT postgis_version()`

// DatabaseProbe reads installation metadata through its owner and verifies that PostGIS is usable.
func DatabaseProbe(pool *pgxpool.Pool, readMetadata ReadReadyMetadata) ReadinessProbe {
	return func(ctx context.Context) (servedapi.ReadyStatus, error) {
		metadata, err := readMetadata(ctx)
		if err != nil {
			return servedapi.ReadyStatus{}, err
		}
		var postgisVersion string
		if err = pool.QueryRow(ctx, postGISVersionSelection).Scan(&postgisVersion); err != nil {
			return servedapi.ReadyStatus{}, err
		}
		return servedapi.ReadyStatus{
			City:     servedapi.ReadyStatusCity(metadata.City),
			Currency: servedapi.ReadyStatusCurrency(metadata.Currency),
			Timezone: servedapi.ReadyStatusTimezone(metadata.Timezone),
		}, nil
	}
}

// MailReadinessProbe reports whether the store the mail stub serves is usable. It answers with the
// failure rather than with a description of one, because the only thing readiness decides is whether
// the container may be given work.
type MailReadinessProbe func(context.Context) error
