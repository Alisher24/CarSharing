package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrScenarioVehicleInUse refuses a restoration that would reach a person's own rental. The whole
// command stops: putting some vehicles back and leaving others would leave the demonstration in a
// state nobody declared, and overwriting the rental would destroy what the person did.
var ErrScenarioVehicleInUse = errors.New(
	"restoration refused: a person has rented a scenario vehicle")

// Restore puts the prepared vehicles and their rentals back to the state the demonstration starts
// from. It touches only the vehicles the scenario declares: the ones left free for a person to book
// by hand, and every account outside the scenario, are not read from and not written to.
//
// Seeding creates what is missing; this command is the only one that returns what already exists.
func Restore(ctx context.Context, pool *pgxpool.Pool) error {
	restoration := restorer{store: store{pool: pool}, users: auth.NewUserStore(pool)}
	return database.InTransaction(ctx, pool, restoration.restore)
}

type restorer struct {
	store store
	users *auth.UserStore
}

func (r restorer) restore(ctx context.Context) error {
	scenario := scenarioVehicles()
	vehicleIDs := identifiersOf(scenario)

	// The vehicles are locked before anything is read, so a rental created while this runs either
	// finishes before the conflict check sees it or waits until the restoration has committed.
	if err := r.store.lockScenarioVehicles(ctx, vehicleIDs); err != nil {
		return err
	}
	if err := r.refuseVehiclesRentedByPeople(ctx, scenario, vehicleIDs); err != nil {
		return err
	}
	if err := r.store.deleteScenarioRentals(ctx, vehicleIDs); err != nil {
		return err
	}
	for _, vehicle := range scenario {
		if err := r.store.restoreVehicle(ctx, vehicle); err != nil {
			return err
		}
	}
	return r.restoreRentals(ctx, scenario)
}

func (r restorer) refuseVehiclesRentedByPeople(
	ctx context.Context, scenario []Vehicle, vehicleIDs []string,
) error {
	inUse, err := r.store.vehiclesRentedByPeople(ctx, vehicleIDs, scenarioAddressesOf(scenario))
	if err != nil {
		return err
	}
	if len(inUse) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrScenarioVehicleInUse, strings.Join(inUse, ", "))
}

func (r restorer) restoreRentals(ctx context.Context, scenario []Vehicle) error {
	zone, tariff := Zone(), Tariff()
	for _, vehicle := range scenario {
		if vehicle.HeldBy == rentals.NotHeld {
			continue
		}
		owner, err := r.scenarioAccount(ctx, vehicle.ScenarioAccount)
		if err != nil {
			return err
		}
		if err = r.store.insertRental(ctx, vehicle.preparedRental(owner, tariff.ID, zone.ID)); err != nil {
			return err
		}
	}
	return nil
}

func (r restorer) scenarioAccount(ctx context.Context, address string) (uuid.UUID, error) {
	email, err := auth.ParseEmail(address)
	if err != nil {
		return uuid.UUID{}, err
	}
	user, _, err := r.users.ByEmail(ctx, email)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("scenario account %s is missing; seed first: %w", address, err)
	}
	return user.ID, nil
}

// scenarioVehicles are the vehicles the command puts back: the ones a prepared rental holds and the
// ones that stand as permanent examples of an exhausted reserve.
func scenarioVehicles() []Vehicle {
	var scenario []Vehicle
	for _, vehicle := range Fleet() {
		if vehicle.Restored() {
			scenario = append(scenario, vehicle)
		}
	}
	return scenario
}

func identifiersOf(vehicles []Vehicle) []string {
	identifiers := make([]string, 0, len(vehicles))
	for _, vehicle := range vehicles {
		identifiers = append(identifiers, vehicle.ID)
	}
	return identifiers
}

// scenarioAddressesOf lists the accounts a prepared rental may belong to. A rental on a scenario
// vehicle held by any other account is a person's, and is what the command refuses to touch.
func scenarioAddressesOf(vehicles []Vehicle) []string {
	var addresses []string
	for _, vehicle := range vehicles {
		if vehicle.ScenarioAccount != "" {
			addresses = append(addresses, vehicle.ScenarioAccount)
		}
	}
	return addresses
}
