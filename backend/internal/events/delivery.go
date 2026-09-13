package events

import (
	"context"
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/outbox"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Deliveries is what the worker runs for every signal task it claims. A task whose kind this build
// does not announce has no delivery here, so it is kept with its error instead of being reported as
// delivered.
func Deliveries(pool *pgxpool.Pool) outbox.Deliveries {
	deliveries := make(outbox.Deliveries, len(AnnouncedKinds))
	publish := signalPublisher{pool: pool}
	for _, kind := range AnnouncedKinds {
		deliveries[string(kind)] = publish.deliver
	}
	return deliveries
}

// signalPublisher announces a signal to every API process listening on the channel. The
// notification is not durable: losing one loses nothing but the promptness of a client's refresh,
// which the REST snapshot repairs.
type signalPublisher struct{ pool *pgxpool.Pool }

// notifyStatement publishes one notification. It is a statement of its own, so it commits the moment
// it runs: the task's row lock was released before the delivery, and nothing else is held here.
const notifyStatement = `SELECT pg_notify($1, $2)`

func (p signalPublisher) deliver(ctx context.Context, task outbox.Task) error {
	signal := Signal{
		Kind:       Kind(task.Kind),
		ResourceID: task.ResourceID,
		Version:    task.Version,
		Recipient:  task.Recipient,
	}
	payload, err := signal.encoded()
	if err != nil {
		return err
	}
	if _, err = database.QuerierFrom(ctx, p.pool).Exec(ctx, notifyStatement, Channel, string(payload)); err != nil {
		return fmt.Errorf("signal could not be published: %w", err)
	}
	return nil
}
