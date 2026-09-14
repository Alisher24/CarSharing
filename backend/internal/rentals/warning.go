package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// warning creates the warning of a reservation that has entered the minute before its deadline. It
// is the rentals module's own transition for the same reason the expiry is: what the record tells
// about is a reservation, and the module that owns the stages owns what a stage means.
type warning struct{ pool *pgxpool.Pool }

func newWarning(pool *pgxpool.Pool) *warning { return &warning{pool: pool} }

// dueWarnings finds the reservations whose last minute is due. The selection takes no lock: a
// reservation an equally timed pass or a command has dealt with since is left alone by the
// transition below, which is where the decision is made rather than here.
//
// The window is stated with the lead the module declares, so "one minute" stands in one place, and
// both of its ends are read from the database clock: the moment a pass acts on is the database's,
// never this process's.
const dueWarningsStatement = `
SELECT id
FROM rentals
WHERE stage = $1
  AND expires_at > clock_timestamp()
  AND expires_at - make_interval(secs => $2) <= clock_timestamp()
ORDER BY expires_at, id`

func dueWarnings(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, dueWarningsStatement,
		stage.Reserved, WarningLead.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var due []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		due = append(due, id)
	}
	return due, rows.Err()
}

// warnDue creates the warning of every reservation inside its last minute and reports how many it
// created.
//
// Each reservation is warned in its own transaction under the shared lock order, so a pass and a
// command arriving at the same moment wait for each other rather than taking the accounts and
// vehicles of the fleet in two different orders. A reservation another pass or a read has already
// warned is counted as not warned by this pass: the warning was created once.
func (w *warning) warnDue(ctx context.Context) (int64, error) {
	due, err := dueWarnings(ctx, w.pool)
	if err != nil {
		return 0, err
	}
	var warned int64
	for _, id := range due {
		created, err := w.warn(ctx, id)
		if err != nil {
			return warned, err
		}
		if created {
			warned++
		}
	}
	return warned, nil
}

// warn creates the warning of one reservation that was inside its last minute when the pass read it.
// Whether it still is decides the transition, and that is judged after the locks with the moment the
// transaction fixed: a wait for those locks cannot put a warning past a deadline, and a rental that
// has left reserved in the meantime is left alone.
func (w *warning) warn(ctx context.Context, id string) (bool, error) {
	var created bool
	err := transact(ctx, w.pool, rentalParticipants(w.pool, id),
		func(txCtx context.Context, moment time.Time) error {
			held, err := rentalByID(txCtx, w.pool, id)
			if errors.Is(err, ErrRentalNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			created, err = createDueWarning(txCtx, w.pool, held, moment)
			return err
		})
	return created, err
}

// createDueWarning writes the warning of one reservation whose last minute has begun at the moment
// the transaction fixed, and records the personal signal with it. It reports whether this call
// created the warning: the unique key on the rental and the kind decides, so a repeated pass, a
// second worker and a read that arrives after the sweep all leave the stored notification alone.
//
// The moment is the one the transaction read from the database after its locks, so a warning is
// created at the moment the reservation was judged to be inside the window rather than at the moment
// a request happened to arrive.
func createDueWarning(
	ctx context.Context, pool *pgxpool.Pool, held Rental, moment time.Time,
) (bool, error) {
	if !held.WarningDue(moment) {
		return false, nil
	}
	_, created, err := notifications.NewStore(pool).Create(ctx, notifications.About{
		UserID:   held.UserID,
		RentalID: held.ID,
		Kind:     notifications.ReservationExpiring,
	}, moment)
	return created, err
}

// deactivateWarning makes the warning of a rental that has left the reserved stage inactive, in the
// transaction that moved the rental: the change to the notification and the personal signal that
// announces it are written with the transition, so no reader can see a warning that is still current
// for a reservation that has ended.
//
// Nothing here creates a warning. A reservation that ended before its last minute never had one, and
// a belated warning does not exist.
func deactivateWarning(ctx context.Context, pool *pgxpool.Pool, released Rental) error {
	_, _, err := notifications.NewStore(pool).Deactivate(
		ctx, released.ID, notifications.ReservationExpiring)
	return err
}
