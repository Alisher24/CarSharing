package rentals

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadNotifications reads notification-owned records inside the rental transaction that first fixes
// a warning that has become due. The composition root supplies the read, so neither module learns the
// other's record types.
type ReadNotifications func(context.Context, time.Time) error

// WithNotificationRead fixes a due reservation warning and performs the supplied notification read
// under the same locks and transaction.
func (s *Service) WithNotificationRead(
	ctx context.Context,
	caller uuid.UUID,
	read ReadNotifications,
) error {
	return transact(
		ctx,
		s.pool,
		notificationParticipants(s.pool, caller),
		func(txCtx context.Context, tx pgx.Tx, moment time.Time) error {
			held, err := liveRentalAt(
				txCtx,
				s.pool,
				s.vehicles,
				s.warnings,
				tx,
				moment,
				userLiveRentalSelection,
				caller,
			)
			if err != nil {
				return err
			}
			if held != nil {
				if _, err = createDueWarning(txCtx, s.warnings, *held, moment); err != nil {
					return err
				}
			}
			return read(txCtx, moment)
		},
	)
}

func notificationParticipants(
	pool *pgxpool.Pool,
	caller uuid.UUID,
) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		planned := participants{users: []uuid.UUID{caller}}
		held, err := liveRentalOf(ctx, pool, userLiveRentalSelection, caller)
		if err != nil || held == nil {
			return planned, err
		}
		planned.rentals = sortedIdentifiers(held.ID)
		return planned, nil
	}
}
