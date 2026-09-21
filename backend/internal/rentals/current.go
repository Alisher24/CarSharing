package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Current is what an account is doing at one moment: the rental that holds a vehicle, that vehicle
// as the catalog publishes it, the moment the read fixed and the day's allowance.
type Current struct {
	Moment time.Time

	// Rental is the live rental of the account, or nil when it holds none. A cancelled, expired or
	// completed rental is never current: it has released its vehicle and is history.
	Rental *Rental

	// Vehicle is the vehicle the live rental holds. It is absent when there is no live rental.
	Vehicle fleet.Vehicle

	// Progress is what a ride that has begun has taken by the moment of the read. A reservation
	// carries the zero value, which is not published.
	Progress billing.Charge

	// Limit is the day's allowance, which is answered whether or not a rental is current.
	Limit DailyLimit
}

// Current answers what one account is doing now, and records the expiry of a reservation whose
// deadline has passed before answering.
//
// The read is not a pure projection: a reservation that has run out is still reserved in the
// database until something moves it, and a client that is told there is nothing current must be
// told so about a vehicle that has actually been released. The transition is the one the sweep
// performs, reached through the same lock order, so a read and a sweep arriving together produce
// one transition rather than two.
//
// The model of the vehicle is brought to the moment of the read for the same reason. A ride whose
// sources have run out is over before the answer is written, and the answer is then the one a client
// can act on: no current rental, and an ending it can open from the report of it. The transition is
// the one the simulator performs, reached through the same lock order and the same mechanism, so a
// read and a tick arriving together produce one ending rather than two.
func (s *Service) Current(ctx context.Context, caller uuid.UUID) (Current, error) {
	var current Current
	err := transact(
		ctx,
		s.pool,
		currentParticipants(s.pool, caller),
		func(txCtx context.Context, tx pgx.Tx, moment time.Time) error {
			var err error
			current, err = s.currentAt(txCtx, tx, caller, moment)
			return err
		},
	)
	if err != nil {
		return Current{}, err
	}
	return current, nil
}

func (s *Service) currentAt(
	ctx context.Context,
	tx pgx.Tx,
	caller uuid.UUID,
	moment time.Time,
) (Current, error) {
	held, err := s.currentRentalAt(ctx, tx, caller, moment)
	if err != nil {
		return Current{}, err
	}
	limit, err := readDailyLimit(ctx, s.pool, caller, moment)
	if err != nil {
		return Current{}, err
	}
	current := Current{Moment: moment, Limit: limit, Rental: held}
	if held == nil {
		return current, nil
	}
	if _, err = createDueWarning(ctx, s.warnings, *held, moment); err != nil {
		return Current{}, err
	}
	current.Vehicle, err = s.vehicles.VehicleAt(ctx, held.VehicleID, moment)
	if err != nil {
		return Current{}, err
	}
	if held.Riding() {
		current.Progress, err = readProgress(ctx, s.pool, *held, moment)
	}
	return current, err
}

func (s *Service) currentRentalAt(
	ctx context.Context,
	tx pgx.Tx,
	caller uuid.UUID,
	moment time.Time,
) (*Rental, error) {
	held, err := liveRentalAt(
		ctx,
		s.pool,
		s.vehicles,
		s.warnings,
		tx,
		moment,
		userLiveRentalSelection,
		caller,
	)
	if err != nil || held == nil || !held.Riding() {
		return held, err
	}
	if _, err = s.reconcileVehicle(ctx, tx, moment, held.VehicleID, held); err != nil {
		return nil, err
	}
	return liveRentalAt(
		ctx,
		s.pool,
		s.vehicles,
		s.warnings,
		tx,
		moment,
		userLiveRentalSelection,
		caller,
	)
}

// currentParticipants is the rows this read touches: the account, and the vehicle and rental that
// currently hold one for it.
func currentParticipants(pool *pgxpool.Pool, caller uuid.UUID) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		planned := participants{users: []uuid.UUID{caller}}
		held, err := liveRentalOf(ctx, pool, userLiveRentalSelection, caller)
		if err != nil {
			return participants{}, err
		}
		if held == nil {
			return planned, nil
		}
		planned.vehicles = []string{held.VehicleID}
		planned.rentals = sortedIdentifiers(held.ID)
		return planned, nil
	}
}
