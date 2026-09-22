package main

import (
	"net/http"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/demo"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/Alisher24/CarSharing/backend/internal/zones"
	"github.com/jackc/pgx/v5/pgxpool"
)

// assemble builds everything this process serves: the readiness probe, the session store, the account
// rules, the rental module with the model of the fleet, and both the public and the internal surface,
// all over the one pool so that a request can commit a user and its session together.
func assemble(cfg config.Config, pool *pgxpool.Pool, hub *events.Hub) (assembled, error) {
	modules, err := newAPIModules(cfg, pool)
	if err != nil {
		return assembled{}, err
	}
	public, err := publicHandler(cfg, pool, hub, modules)
	if err != nil {
		return assembled{}, err
	}
	internal, err := internalHandler(cfg, modules.reservations)
	if err != nil {
		return assembled{}, err
	}
	confirmations, err := demo.NewConfirmations(pool, modules.vehicles, modules.models)
	if err != nil {
		return assembled{}, err
	}
	return assembled{
		handler:       httpapi.NewSurfaceRouter(public, internal),
		confirmations: confirmations,
	}, nil
}

func newAPIModules(cfg config.Config, pool *pgxpool.Pool) (apiModules, error) {
	users := auth.NewUserStore(pool)
	hasher := auth.NewPasswordHasher(cfg.Argon2)
	authentication, err := auth.NewService(users, hasher)
	if err != nil {
		return apiModules{}, err
	}
	vehicles := fleet.NewStore(pool)
	prices := tariffs.NewStore(pool)
	models := simulation.NewStore(pool)
	issued := invoices.NewStore(pool)
	rentalRecords := rentals.NewStore(pool)
	notificationStore := notifications.NewStore(pool)
	reservations, err := newRentalService(pool, vehicles, prices, issued, notificationStore, models)
	if err != nil {
		return apiModules{}, err
	}
	notificationService, err := notifications.NewService(pool)
	if err != nil {
		return apiModules{}, err
	}
	cursors, err := cursor.NewSigner(cfg.CursorSigningKey)
	if err != nil {
		return apiModules{}, err
	}
	return apiModules{
		users:               users,
		authentication:      authentication,
		vehicles:            vehicles,
		prices:              prices,
		models:              models,
		issued:              issued,
		rentalRecords:       rentalRecords,
		reservations:        reservations,
		notificationRecords: notificationStore,
		notificationReads:   notificationService,
		cursors:             cursors,
	}, nil
}

func newRentalService(
	pool *pgxpool.Pool,
	vehicles *fleet.Store,
	prices *tariffs.Store,
	issued *invoices.Store,
	notificationStore *notifications.Store,
	models *simulation.Store,
) (*rentals.Service, error) {
	return rentals.NewService(
		pool,
		vehicles,
		prices,
		issued,
		rentals.WarningOperations{
			Create: notificationStore.CreateReservationWarning,
			End:    notificationStore.EndReservationWarning,
		},
		completionRecorder(notifications.NewCompleter(pool)),
		models,
	)
}

func publicHandler(
	cfg config.Config,
	pool *pgxpool.Pool,
	hub *events.Hub,
	modules apiModules,
) (http.Handler, error) {
	return httpapi.NewHandler(httpapi.Dependencies{
		Probe:          httpapi.DatabaseProbe(pool, readinessMetadata(modules.rentalRecords)),
		AllowedOrigins: cfg.AllowedOrigins,
		Pool:           pool,
		Sessions:       sessions.NewManager(pool, cfg.SessionCookieSecure),
		Auth:           modules.authentication,
		Users:          modules.users,
		Throttle:       auth.NewThrottle(pool, cfg.RateLimits),
		Events:         hub,
		StreamTiming:   events.DefaultStreamTiming(),
		Reservations:   modules.reservations,
		Notifications: notificationOperations{
			reservations: modules.reservations,
			collection:   modules.notificationRecords,
			reads:        modules.notificationReads,
		},
		Invoices: modules.issued,
		Cursors:  modules.cursors,
		Catalog: httpapi.Catalog{
			Vehicles: modules.vehicles,
			Zones:    zones.NewStore(pool),
			Tariffs:  modules.prices,
		},
	})
}

func internalHandler(cfg config.Config, reservations *rentals.Service) (http.Handler, error) {
	return httpapi.NewInternalHandler(httpapi.InternalDependencies{
		Simulation: reservations,
		Demo:       reservations,
		Tokens: httpapi.InternalTokens{
			Simulator:   cfg.SimulatorToken,
			DemoControl: cfg.DemoControlToken,
		},
		Demonstrating: cfg.Environment.Name == config.DemoEnvironment,
	})
}
