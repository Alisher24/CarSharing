package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/google/uuid"
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
func (s *Service) Current(ctx context.Context, caller uuid.UUID) (Current, error) {
	var current Current
	err := transact(ctx, s.pool, currentParticipants(s.pool, caller),
		func(txCtx context.Context, moment time.Time) error {
			held, err := liveRentalAt(txCtx, s.pool, moment, userLiveRentalSelection, caller)
			if err != nil {
				return err
			}
			limit, err := readDailyLimit(txCtx, s.pool, caller, moment)
			if err != nil {
				return err
			}
			current = Current{Moment: moment, Limit: limit}
			if held == nil {
				return nil
			}
			// The read fixes a warning the worker missed through the transition the worker performs,
			// as it already fixes an expiry the worker missed: a person whose worker was stopped
			// still learns about the last minute when they open the application.
			if _, err = createDueWarning(txCtx, s.pool, *held, moment); err != nil {
				return err
			}
			vehicle, err := s.vehicles.VehicleAt(txCtx, held.VehicleID, moment)
			if err != nil {
				return err
			}
			current.Rental = held
			current.Vehicle = vehicle
			return nil
		})
	if err != nil {
		return Current{}, err
	}
	return current, nil
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
