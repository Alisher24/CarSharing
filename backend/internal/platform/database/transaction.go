// Transactions and the reader/writer handle the stores use. A store never chooses between the
// pool and a transaction itself: it asks the context, so work started inside a transaction cannot
// quietly commit outside it.

package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the subset of the pgx API this application uses to read and write. Both the pool and
// a transaction satisfy it, so a store can be handed either without knowing which it received.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// transactionKey carries the transaction of the request being served.
type transactionKey struct{}

// WithTransaction marks a context as running inside a transaction, so that a store reached further
// down commits with it rather than beside it.
func WithTransaction(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, transactionKey{}, tx)
}

// QuerierFrom returns the transaction the context carries, or the pool when it carries none. A
// store uses it for every statement so that work inside a transaction cannot escape it: this is
// what makes a rolled back registration leave neither a user row nor a session row behind.
func QuerierFrom(ctx context.Context, pool *pgxpool.Pool) Querier {
	if tx, ok := ctx.Value(transactionKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}

// InTransaction runs work in one transaction, rolling back on error or panic. The commit error is
// returned to the caller, because work that cannot commit has not happened.
func InTransaction(ctx context.Context, pool *pgxpool.Pool, work func(context.Context) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful commit is a no-op, so one deferred call covers the error path,
	// the panic path and the ordinary path alike.
	defer func() { _ = tx.Rollback(ctx) }()
	if err = work(WithTransaction(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
