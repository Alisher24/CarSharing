package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ReadOne rejects a selection that returns more than one row and maps absence to the owner's error.
func ReadOne[T any](
	ctx context.Context,
	querier Querier,
	scan func(pgx.Row, *T) error,
	missing error,
	statement string,
	arguments ...any,
) (T, error) {
	var found T
	rows, err := querier.Query(ctx, statement, arguments...)
	if err != nil {
		return found, err
	}
	found, err = pgx.CollectExactlyOneRow(rows, func(row pgx.CollectableRow) (T, error) {
		var value T
		err := scan(row, &value)
		return value, err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return found, missing
	}
	return found, err
}
