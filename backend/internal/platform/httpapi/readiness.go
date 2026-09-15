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

// MailReadinessProbe reports whether the store the mail stub serves is usable. It answers with the
// failure rather than with a description of one, because the only thing readiness decides is whether
// the container may be given work.
type MailReadinessProbe func(context.Context) error

// countMessagesStatement counts the letters of the box. It reads the mail schema rather than the
// connection, which is what readiness means for this process: a stub that answers ready without
// reaching its own schema turns an unreachable store into a delivery the worker can never complete
// instead of into a container that is not ready.
const countMessagesStatement = `SELECT count(*) FROM mailstub.messages`

// MailDatabaseProbe reads the mail box. Counting rather than selecting a row keeps an empty box ready:
// there is nothing to find, and an empty box is exactly what a running stub starts with.
func MailDatabaseProbe(pool *pgxpool.Pool) MailReadinessProbe {
	return func(ctx context.Context) error {
		var stored int64
		return pool.QueryRow(ctx, countMessagesStatement).Scan(&stored)
	}
}
