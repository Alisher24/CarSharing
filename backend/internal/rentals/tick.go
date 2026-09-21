package rentals

import (
	"context"
	"sort"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TickCommand is one call of the simulator: the identifier of the call, which is what a repeated call
// is recognised by, and how its answer is spelled.
type TickCommand struct {
	TickID  string
	Attempt Attempt
}

// TickOutcome is what one call changed: the vehicles that moved or spent something and the rides that
// ran out.
type TickOutcome struct {
	TickID           string
	Moment           time.Time
	ChangedVehicles  []string
	CompletedRentals []string
}

// Tick advances every simulated vehicle to one moment of the database clock, in one transaction.
//
// The whole fleet is advanced together rather than vehicle by vehicle, because a tick that committed
// some vehicles and rolled back others would leave the fleet describing moments that no single call
// ever asked about. A call the simulator repeats after losing its answer is answered with the result
// the first call stored, so a lost response costs no second movement and no second ending.
func (s *Service) Tick(ctx context.Context, command TickCommand) (Answered, error) {
	return s.answer(ctx, idempotency.ForInstallation(), command.Attempt,
		s.tickParticipants,
		func(ctx context.Context, tx pgx.Tx, moment time.Time) (Outcome, error) {
			return s.tickWithin(ctx, tx, moment, command)
		})
}

// tickWithin advances the fleet with the participants locked and the moment fixed.
func (s *Service) tickWithin(
	ctx context.Context,
	tx pgx.Tx,
	moment time.Time,
	command TickCommand,
) (Outcome, error) {
	outcomes, err := s.reconcileFleet(ctx, tx, moment)
	if err != nil {
		return Outcome{}, err
	}
	tick := TickOutcome{TickID: command.TickID, Moment: moment}
	for _, outcome := range outcomes {
		if outcome.Moved {
			tick.ChangedVehicles = append(tick.ChangedVehicles, outcome.VehicleID)
		}
		if outcome.Ended != nil {
			tick.CompletedRentals = append(tick.CompletedRentals, outcome.Ended.ID)
		}
	}
	return Outcome{Moment: moment, Tick: tick}, nil
}

// tickParticipants is every row a tick touches: every vehicle the model travels, every rental that
// still holds one and the accounts those rentals belong to. A tick advances the whole fleet, so it
// plans the whole fleet's locks rather than a subset a later tick would have to meet.
func (s *Service) tickParticipants(ctx context.Context) (participants, error) {
	vehicles, err := s.vehicles.Simulated(ctx)
	if err != nil {
		return participants{}, err
	}
	held, err := liveRentals(ctx, s.pool)
	if err != nil {
		return participants{}, err
	}

	planned := participants{
		vehicles: make([]string, 0, len(vehicles)),
		rentals:  make([]string, 0, len(held)),
		users:    make([]uuid.UUID, 0, len(held)),
	}
	for _, vehicle := range vehicles {
		planned.vehicles = append(planned.vehicles, vehicle.ID)
	}
	for index := range held {
		planned.rentals = append(planned.rentals, held[index].ID)
		planned.users = append(planned.users, held[index].UserID)
	}
	sort.Strings(planned.vehicles)
	sort.Strings(planned.rentals)
	planned.users = distinctUsers(planned.users)
	return planned, nil
}
