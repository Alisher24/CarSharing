package database

import (
	"context"
	"time"
)

// Moment reads the database wall clock through the caller's connection or transaction.
func Moment(ctx context.Context, querier Querier) (time.Time, error) {
	var moment time.Time
	err := querier.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&moment)
	return moment, err
}
