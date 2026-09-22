package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dueWarnings names the reservations that have entered the minute before their deadline, which is what
// the sweep hands to the warning transition.
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
func (s *reservationSweep) warnDue(ctx context.Context) (int64, error) {
	return s.sweepDue(ctx, dueWarnings, s.warn)
}

// warn creates the warning of one reservation that was inside its last minute when the pass read it.
// Whether it still is decides the transition, and that is judged with the moment the transaction fixed:
// a wait for the locks cannot put a warning past a deadline, and a rental that has left reserved in the
// meantime is left alone.
func (s *reservationSweep) warn(
	ctx context.Context, _ pgx.Tx, moment time.Time, held Rental,
) (bool, error) {
	return createDueWarning(ctx, s.warnings, held, moment)
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
	ctx context.Context,
	operations WarningOperations,
	held Rental,
	moment time.Time,
) (bool, error) {
	if !held.WarningDue(moment) {
		return false, nil
	}
	return operations.Create(ctx, held.UserID, held.ID, moment)
}

// deactivateWarning makes the warning of a rental that has left the reserved stage inactive, in the
// transaction that moved the rental: the change to the notification and the personal signal that
// announces it are written with the transition, so no reader can see a warning that is still current
// for a reservation that has ended.
//
// Nothing here creates a warning. A reservation that ended before its last minute never had one, and
// a belated warning does not exist.
func deactivateWarning(
	ctx context.Context,
	operations WarningOperations,
	released Rental,
) error {
	return operations.End(ctx, released.ID)
}
