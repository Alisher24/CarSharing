// Command api serves the public HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/demo"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/lifecycle"
	"github.com/Alisher24/CarSharing/backend/internal/platform/periodic"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/Alisher24/CarSharing/backend/internal/zones"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// readHeaderTimeout bounds how long a client may take to send the request headers, which is what
	// keeps a connection that never finishes a request from holding a slot.
	readHeaderTimeout = 5 * time.Second

	// readTimeout and writeTimeout bound one whole request and its response. A tick carries a batch,
	// so the response side is given the same ten seconds as the request side.
	readTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second

	// idleTimeout is how long a keep-alive connection may sit unused before it is closed.
	idleTimeout = 60 * time.Second

	// maxHeaderBytes is the largest request header block the server reads.
	maxHeaderBytes = 16 << 10
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// assembled is what one API process runs: the handler both of its surfaces are served through, and the
// demonstration source of telemetry it confirms the fleet with.
type assembled struct {
	handler       http.Handler
	confirmations *demo.Confirmations
}

// assemble builds everything this process serves: the readiness probe, the session store, the account
// rules, the rental module with the model of the fleet, and both the public and the internal surface,
// all over the one pool so that a request can commit a user and its session together.
func assemble(cfg config.Config, pool *pgxpool.Pool, hub *events.Hub) (assembled, error) {
	users := auth.NewUserStore(pool)
	hasher := auth.NewPasswordHasher(cfg.Argon2)
	service, err := auth.NewService(users, hasher)
	if err != nil {
		return assembled{}, err
	}
	vehicles := fleet.NewStore(pool)
	prices := tariffs.NewStore(pool)
	models := simulation.NewStore(pool)
	issued := invoices.NewStore(pool)
	reservations, err := rentals.NewService(pool, vehicles, prices, issued,
		notifications.NewCompleter(pool), models)
	if err != nil {
		return assembled{}, err
	}
	notifications, err := notifications.NewService(pool)
	if err != nil {
		return assembled{}, err
	}
	cursors, err := cursor.NewSigner(cfg.CursorSigningKey)
	if err != nil {
		return assembled{}, err
	}

	public, err := httpapi.NewHandler(httpapi.Dependencies{
		Probe:          httpapi.DatabaseProbe(pool),
		AllowedOrigins: cfg.AllowedOrigins,
		Pool:           pool,
		Sessions:       sessions.NewManager(pool, cfg.SessionCookieSecure),
		Auth:           service,
		Users:          users,
		Throttle:       auth.NewThrottle(pool, cfg.RateLimits),
		Events:         hub,
		Reservations:   reservations,
		Notifications:  notificationOperations{reservations: reservations, reads: notifications},
		Invoices:       issued,
		Cursors:        cursors,
		Catalog: httpapi.Catalog{
			Vehicles: vehicles,
			Zones:    zones.NewStore(pool),
			Tariffs:  prices,
		},
	})
	if err != nil {
		return assembled{}, err
	}
	internal, err := httpapi.NewInternalHandler(httpapi.InternalDependencies{
		Simulation: reservations,
		Demo:       reservations,
		Tokens: httpapi.InternalTokens{
			Simulator:   cfg.SimulatorToken,
			DemoControl: cfg.DemoControlToken,
		},
		Demonstrating: cfg.Environment.Name == config.DemoEnvironment,
	})
	if err != nil {
		return assembled{}, err
	}
	return assembled{
		handler:       httpapi.NewSurfaceRouter(public, internal),
		confirmations: demo.NewConfirmations(pool, models),
	}, nil
}

// notificationOperations is the two halves of the notification surface as one dependency: the
// collection belongs to the rentals module, because reading it fixes a warning that has become due
// for the reservation the reader holds, and the read of one notification belongs to the
// notifications module. The composition root is where the two are joined; neither module learns
// about the other's records.
type notificationOperations struct {
	reservations *rentals.Service
	reads        *notifications.Service
}

func (o notificationOperations) Collection(
	ctx context.Context, caller uuid.UUID, after *notifications.Position, limit int,
) (rentals.NotificationPage, error) {
	return o.reservations.Collection(ctx, caller, after, limit)
}

func (o notificationOperations) MarkRead(
	ctx context.Context, owner uuid.UUID, id string,
) (notifications.Result, error) {
	return o.reads.MarkRead(ctx, owner, id)
}

// startBackgroundWork starts the recurring work this process owns. Listening for published signals
// is how a stream learns that something changed, so it always runs. The demonstration telemetry
// source stands in for vehicles that do not exist outside a demonstration, so it runs only where the
// demonstration does. Releasing reservations that have run out belongs to the worker process, which
// is the one place that decides a deadline has passed.
func startBackgroundWork(
	ctx context.Context, cfg config.Config, hub *events.Hub, confirmations *demo.Confirmations,
) {
	go hub.Run(ctx)
	if cfg.Environment.Name != config.DemoEnvironment {
		return
	}
	go periodic.Run(ctx, "demonstration telemetry", demo.ConfirmationInterval, confirmations.Confirm)
}

// newServer is the HTTP server this process runs, with the timeouts a publicly reachable listener
// needs: a request that stalls at any stage is given up on rather than held.
func newServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startup, cancel := context.WithTimeout(ctx, database.DatabaseStartupTimeout)
	pool, err := database.Open(startup, cfg.Database)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	hub := events.NewHub(pool)
	serving, err := assemble(cfg, pool, hub)
	if err != nil {
		return err
	}
	startBackgroundWork(ctx, cfg, hub, serving.confirmations)
	return serve(ctx, newServer(cfg, serving.handler))
}

// serve answers requests until the server fails or the context is cancelled, and then gives the
// in-flight requests their shutdown budget.
func serve(ctx context.Context, server *http.Server) error {
	serverExit := make(chan error, 1)
	go func() { serverExit <- server.ListenAndServe() }()
	slog.Info("API started", "address", server.Addr)
	select {
	case err := <-serverExit:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server failed: %w", err)
		}
	case <-ctx.Done():
		shutdown, done := context.WithTimeout(context.Background(), lifecycle.ShutdownTimeout)
		defer done()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return errors.New("HTTP shutdown deadline exceeded")
		}
	}
	slog.Info("API stopped")
	return nil
}
