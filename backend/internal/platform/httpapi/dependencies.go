package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dependencies is everything the served application needs from the process around it. The process
// that builds them supplies all of them; a caller that serves only what needs no account — an
// isolated contract router, or a routing test — supplies the probe and the catalog and is served
// by NewAnonymousRouter instead.
type Dependencies struct {
	// Probe answers whether the dependencies this deployment needs are usable. It is the only
	// dependency the health operations have.
	Probe ReadinessProbe

	// AllowedOrigins are the browser origins a mutation may come from. An origin outside this set
	// is refused before the request can create an account, a session or a cookie.
	AllowedOrigins []string

	Pool     *pgxpool.Pool
	Sessions *sessions.Manager
	Auth     *auth.Service
	Users    *auth.UserStore
	Throttle *auth.Throttle

	// Catalog is what the operations that need no account read. Both routers are given it,
	// because the anonymous one serves those operations too.
	Catalog Catalog
}

// Catalog is the read side of everything a visitor sees without signing in. Each resource is read
// on its own, so one of them failing leaves the other two answerable.
type Catalog struct {
	Vehicles VehicleReader
	Zones    ZoneReader
	Tariffs  TariffReader
}

// catalogHandlers are the operations a visitor reads without an account, each over the reader
// that answers it.
type catalogHandlers struct {
	vehicles
	serviceZones
	prices
}

// newCatalogHandlers builds them, or names the reader that is missing. A reader is refused rather
// than defaulted, because a handler that reached a nil one would answer a request it never read.
func newCatalogHandlers(catalog Catalog) (catalogHandlers, error) {
	for _, required := range []struct {
		name     string
		supplied bool
	}{
		{"vehicle catalog", catalog.Vehicles != nil},
		{"service zones", catalog.Zones != nil},
		{"tariffs", catalog.Tariffs != nil},
	} {
		if !required.supplied {
			return catalogHandlers{}, fmt.Errorf("%w: %s", ErrIncompleteApplication, required.name)
		}
	}
	return catalogHandlers{
		vehicles:     vehicles{reader: catalog.Vehicles},
		serviceZones: serviceZones{reader: catalog.Zones},
		prices:       prices{reader: catalog.Tariffs},
	}, nil
}

// ErrIncompleteApplication refuses to serve an application whose dependencies were not all
// supplied. A service locator that answers a missing dependency at request time panics inside a
// handler instead, which a client reads as a crash rather than as a process that never started.
var ErrIncompleteApplication = errors.New("the HTTP application is missing a dependency")

// server implements every served operation by delegating to the handler that owns its concern, so
// that the generated interface is satisfied in one place without collecting unrelated methods on
// one type.
type server struct {
	health
	accounts
	catalogHandlers
}

// originSet indexes the allowed origins for lookup, so the check is a comparison rather than a scan.
func originSet(origins []string) map[string]bool {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return allowed
}

// refuseCredentials answers an operation the anonymous router does not serve as an
// unauthenticated request: the operation exists in the contract, and this router has no
// credentials to check.
func refuseCredentials(context.Context, *openapi3filter.AuthenticationInput) error {
	return errNoLiveSession
}
