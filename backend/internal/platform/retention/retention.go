// Package retention schedules bounded cleanup of records whose owner declares them expired.
package retention

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/periodic"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	BatchSize = 1000
	Interval  = 10 * time.Second
)

type Delete func(context.Context, *pgxpool.Pool, int) (int64, error)

type Sweep struct {
	Name   string
	Delete Delete
}

func Run(ctx context.Context, pool *pgxpool.Pool, sweeps []Sweep) {
	tasks := make([]func(context.Context), 0, len(sweeps))
	for _, sweep := range sweeps {
		tasks = append(tasks, func(ctx context.Context) { sweep.run(ctx, pool) })
	}
	periodic.RunAll(ctx, tasks...)
}

func (sweep Sweep) run(ctx context.Context, pool *pgxpool.Pool) {
	periodic.Run(ctx, sweep.Name, Interval, func(ctx context.Context) (int64, error) {
		return sweep.Delete(ctx, pool, BatchSize)
	})
}
