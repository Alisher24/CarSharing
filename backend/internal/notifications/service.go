package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Result is one notification together with the moment the operation that produced it fixed: the
// moment the read was stored, or the moment the collection was read.
type Result struct {
	Notification Notification
	Moment       time.Time
}

// Service serves the notification operations over one pool and one store. It is the module's runtime
// rather than a second place that decides what a notification is: every statement belongs to the
// store, and the transaction here is what makes the stored change and the signal that announces it
// commit together.
type Service struct {
	pool  *pgxpool.Pool
	store *Store
}

// ErrIncompleteModule refuses to serve a module whose dependencies were not all supplied.
var ErrIncompleteModule = errors.New("the notifications module is missing a dependency")

func NewService(pool *pgxpool.Pool) (*Service, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: database pool", ErrIncompleteModule)
	}
	return &Service{pool: pool, store: NewStore(pool)}, nil
}

// momentStatement reads the moment one operation acts on, which is the database's own clock: a read
// moment the client could choose would not be the moment the change happened.
const momentStatement = `SELECT clock_timestamp()`

// MarkRead stores the read of one notification of this account. The moment is read from the database
// inside the transaction that stores it, so two reads racing for the same notification wait for each
// other rather than overwriting each other's moment, and the personal signal commits with the change.
//
// A notification of another account and one that does not exist are both reported as absent, so a
// caller cannot use the answer to learn that somebody else's notification exists.
func (s *Service) MarkRead(ctx context.Context, owner uuid.UUID, id string) (Result, error) {
	var result Result
	err := database.InTransaction(ctx, s.pool, func(txCtx context.Context) error {
		moment, err := readMoment(txCtx, s.pool)
		if err != nil {
			return err
		}
		stored, _, err := s.store.MarkRead(txCtx, owner, id, moment)
		if err != nil {
			return err
		}
		result = Result{Notification: stored, Moment: moment}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func readMoment(ctx context.Context, pool *pgxpool.Pool) (time.Time, error) {
	var moment time.Time
	err := database.QuerierFrom(ctx, pool).QueryRow(ctx, momentStatement).Scan(&moment)
	return moment, err
}
