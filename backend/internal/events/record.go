package events

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Record writes the signals of a change that has just been made, through the querier the context
// carries. Called inside the transaction that makes the change, it is what ties the two together:
// a rolled back change leaves no task behind, and a committed one cannot lose it.
func Record(ctx context.Context, pool *pgxpool.Pool, signals ...Signal) error {
	if len(signals) == 0 {
		return nil
	}
	tasks := make([]outbox.Task, 0, len(signals))
	for _, signal := range signals {
		tasks = append(tasks, outbox.Task{
			Kind:       string(signal.Kind),
			ResourceID: signal.ResourceID,
			Version:    signal.Version,
			Recipient:  signal.Recipient,
		})
	}
	return outbox.NewStore(pool).Record(ctx, tasks...)
}
