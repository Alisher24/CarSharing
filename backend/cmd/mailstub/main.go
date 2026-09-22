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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpserver"
	"github.com/jackc/pgx/v5/pgxpool"
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
func assemble(server config.MailstubServer, pool *pgxpool.Pool) (assembled, error) {
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
	box := mailstub.NewStore(pool)
	internal, err := httpapi.NewMailstubInternalListener(httpapi.MailstubInternalDependencies{
		Deliveries: acceptor,
		Actions:    actions,
		Probe:      box.Ready,
		Tokens: httpapi.MailstubTokens{
			Delivery: server.DeliveryToken,
			Demo:     server.DemoToken,
		},
		Demonstrating: server.Environment.Name == config.DemoEnvironment,
	})
	if err != nil {
		return assembled{}, err
	}
	inbox, err := httpapi.NewMailstubInboxListener(httpapi.MailstubInboxDependencies{
		Inbox:   box,
		Cursors: cursors,
	})
	if err != nil {
		return assembled{}, err
	}
	return assembled{
		internal: httpserver.New(server.InternalAddr, internal),
		inbox:    httpserver.New(server.InboxAddr, inbox),
	}, nil
}

func run() error {
	settings, err := config.MailstubServerFromEnvironment()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startup, cancel := context.WithTimeout(ctx, database.DatabaseStartupTimeout)
	pool, err := database.Open(startup, settings.Database)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	serving, err := assemble(settings, pool)
	if err != nil {
		return err
	}
	return httpserver.Serve(ctx, serving.internal, serving.inbox)
}
