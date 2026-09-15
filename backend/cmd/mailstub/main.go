// Command mailstub serves the mail stub: the delivery of one letter under a delivery key, the
// demonstration control that arms the loss of an answer, and the read-only inbox that shows what was
// received.
//
// It is a process of its own over a schema of its own, connected as a role that reaches nothing else,
// and it serves two listeners: the internal one, which the external proxy does not publish, and the
// inbox, which is published on the loopback address of the machine it runs on and nowhere else.
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

	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// shutdownTimeout is how long in-flight requests are given to finish after a signal.
	shutdownTimeout = 10 * time.Second

	// readHeaderTimeout bounds how long a client may take to send the request headers, which is what
	// keeps a connection that never finishes a request from holding a slot.
	readHeaderTimeout = 5 * time.Second

	// readTimeout and writeTimeout bound one whole request and its response.
	readTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second

	// idleTimeout is how long a keep-alive connection may sit unused before it is closed.
	idleTimeout = 60 * time.Second

	// maxHeaderBytes is the largest request header block either listener reads. A letter travels in
	// the body, so the headers stay small on both surfaces.
	maxHeaderBytes = 16 << 10
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// assembled is what the process serves: the two listeners of the mail stub, each with the address it
// is reached on.
type assembled struct {
	internal *http.Server
	inbox    *http.Server
}

// assemble builds everything this process serves over the one pool it was given: the internal
// listener with the delivery operation, the demonstration control and the readiness probe of its own
// schema, and the read-only inbox with the cursors of its own key.
func assemble(
	cfg config.Config, server config.MailstubServer, pool *pgxpool.Pool,
) (assembled, error) {
	acceptor, err := mailstub.NewAcceptor(pool)
	if err != nil {
		return assembled{}, err
	}
	actions, err := mailstub.NewActions(pool)
	if err != nil {
		return assembled{}, err
	}
	cursors, err := cursor.NewSigner(server.CursorSigningKey)
	if err != nil {
		return assembled{}, err
	}
	internal, err := httpapi.NewMailstubInternalListener(httpapi.MailstubInternalDependencies{
		Deliveries: acceptor,
		Actions:    actions,
		Probe:      httpapi.MailDatabaseProbe(pool),
		Tokens: httpapi.MailstubTokens{
			Delivery: server.DeliveryToken,
			Demo:     server.DemoToken,
		},
		Demonstrating: cfg.Environment == config.DemoEnvironment,
	})
	if err != nil {
		return assembled{}, err
	}
	inbox, err := httpapi.NewMailstubInboxListener(httpapi.MailstubInboxDependencies{
		Inbox:   mailstub.NewStore(pool),
		Cursors: cursors,
	})
	if err != nil {
		return assembled{}, err
	}
	return assembled{
		internal: newServer(server.InternalAddr, internal),
		inbox:    newServer(server.InboxAddr, inbox),
	}, nil
}

// newServer is one HTTP server of this process, with the timeouts a listener on the internal network
// needs: a request that stalls at any stage is given up on rather than held.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
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
	settings, err := config.MailstubServerFromEnvironment()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startup, cancel := context.WithTimeout(ctx, database.DatabaseStartupTimeout)
	pool, err := database.Open(startup, cfg)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	serving, err := assemble(cfg, settings, pool)
	if err != nil {
		return err
	}
	return serveAll(ctx, serving)
}

// serveAll answers requests on both listeners until one of them fails or the context is cancelled.
// Each listener is served by the same pair of functions — build a server, serve it — because the two
// surfaces differ in their address and their routes and in nothing else: one process, two listeners,
// one box.
func serveAll(ctx context.Context, serving assembled) error {
	failed := make(chan error, 2)
	for _, server := range []*http.Server{serving.internal, serving.inbox} {
		go func(server *http.Server) { failed <- listen(ctx, server) }(server)
	}
	slog.Info("mail stub started", "internal", serving.internal.Addr, "inbox", serving.inbox.Addr)

	select {
	case err := <-failed:
		shutdownBoth(serving)
		if err != nil {
			return err
		}
	case <-ctx.Done():
		shutdownBoth(serving)
	}
	slog.Info("mail stub stopped")
	return nil
}

// listen serves one listener until it fails or the context is cancelled, and gives its in-flight
// requests the shutdown budget before answering. A listener that stopped because it was asked to is
// not a failure of the process.
func listen(ctx context.Context, server *http.Server) error {
	served := make(chan error, 1)
	go func() { served <- server.ListenAndServe() }()
	select {
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("the listener on %s failed: %w", server.Addr, err)
		}
	case <-ctx.Done():
	}
	return nil
}

// shutdownBoth gives both listeners their in-flight requests before the process leaves. A listener
// whose deadline is exceeded is closed rather than waited for: the process is stopping either way,
// and a request that stalls past the budget is one nobody is waiting for.
func shutdownBoth(serving assembled) {
	shutdown, done := context.WithTimeout(context.Background(), shutdownTimeout)
	defer done()
	for _, server := range []*http.Server{serving.internal, serving.inbox} {
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	}
}
