package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
// Seeding creates what is missing; this command is the only one that returns what already exists. The
// model of every vehicle it puts back is dropped with it, so the next reading of the fleet begins from
// the reserves and the position the restoration installed rather than from what a previous ride made
// of them.
func Restore(
	ctx context.Context,
	pool *pgxpool.Pool,
	stores RestoreStores,
) error {
	scenario := scenarioVehicles()
	accounts, err := scenarioAccounts(ctx, stores.Users, scenario)
	if err != nil {
		return err
	}
	restoration := restorer{
		pool:          pool,
		stores:        stores,
		accounts:      accounts,
		scenarioUsers: scenarioUserIDs(scenario, accounts),
	}
	return rentals.WithScenarioRowsLocked(
		ctx,
		pool,
		identifiersOf(scenario),
		restoration.scenarioUsers,
		func(txCtx context.Context, tx pgx.Tx) error {
			return restoration.restore(txCtx, tx, scenario)
		},
	)
}

type restorer struct {
	pool          *pgxpool.Pool
	stores        RestoreStores
	accounts      map[string]uuid.UUID
	scenarioUsers []uuid.UUID
}

func (r restorer) restore(ctx context.Context, tx pgx.Tx, scenario []Vehicle) error {
	vehicleIDs := identifiersOf(scenario)

	if err := r.refuseVehiclesRentedByPeople(ctx, tx, vehicleIDs); err != nil {
		return err
	}
	if err := r.stores.Rentals.DeleteScenarioRentals(
		ctx,
		tx,
		vehicleIDs,
		preparedRentalIDs(scenario),
	); err != nil {
		return err
	}
	signals, err := r.restoreVehicles(ctx, tx, scenario)
	if err != nil {
		return err
	}
	rentalSignals, err := r.restoreRentals(ctx, tx, scenario)
	if err != nil {
		return err
	}
	if err = r.stores.Models.Forget(ctx, vehicleIDs); err != nil {
		return err
	}
	return events.Record(ctx, r.pool, append(signals, rentalSignals...)...)
}

func (r restorer) refuseVehiclesRentedByPeople(
	ctx context.Context,
	tx pgx.Tx,
	vehicleIDs []string,
) error {
	inUse, err := r.stores.Rentals.VehiclesRentedByPeople(
		ctx,
		tx,
		vehicleIDs,
		r.scenarioUsers,
	)
	if err != nil {
		return err
	}
	if len(inUse) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrScenarioVehicleInUse, strings.Join(inUse, ", "))
}

// restoreVehicles puts every scenario vehicle back and states the public change each one is, because
// a restoration that nobody was told about would leave every connected map showing the state the
// previous person left behind.
func (r restorer) restoreVehicles(
	ctx context.Context,
	tx pgx.Tx,
	scenario []Vehicle,
) ([]events.Signal, error) {
	signals := make([]events.Signal, 0, len(scenario))
	for _, vehicle := range scenario {
		version, err := r.stores.Fleet.Restore(ctx, tx, vehicle.installed())
		if err != nil {
			return nil, err
		}
		signals = append(signals, events.Signal{
			Kind:       events.VehicleChanged,
			ResourceID: vehicle.ID,
			Version:    version,
		})
	}
	return signals, nil
}

// restoreRentals puts the prepared rentals back and states the private change each holder is told.
func (r restorer) restoreRentals(
	ctx context.Context,
	tx pgx.Tx,
	scenario []Vehicle,
) ([]events.Signal, error) {
	zone, tariff := Zone(), Tariff()
	var signals []events.Signal
	for _, vehicle := range scenario {
		if vehicle.HeldBy == stage.NotHeld {
			continue
		}
		owner := r.accounts[vehicle.ScenarioAccount]
		version, err := r.stores.Rentals.RestorePrepared(
			ctx,
			tx,
			vehicle.preparedRental(owner, tariff.ID, zone.ID),
		)
		if err != nil {
			return nil, err
		}
		signals = append(signals, events.Signal{
			Kind:       events.RentalChanged,
			ResourceID: vehicle.PreparedRentalID,
			Version:    version,
			Recipient:  owner,
		})
	}
	return signals, nil
}

// scenarioVehicles are the vehicles the command puts back: every vehicle the demonstration declares.
//
// The model spends a reserve as a vehicle moves, so a vehicle a person booked and drove is no longer
// the vehicle the demonstration declares — its position and its sources have moved on. Putting the
// demonstration back therefore means putting all of it back, including the vehicles left free for a
// person to book, which are the ones a demonstration most often drives. What the command still never
// touches is a person's rental and the history of one: a scenario vehicle another account holds stops
// the whole command rather than being overwritten.
func scenarioVehicles() []Vehicle {
	return Fleet()
}

func identifiersOf(vehicles []Vehicle) []string {
	identifiers := make([]string, 0, len(vehicles))
	for _, vehicle := range vehicles {
		identifiers = append(identifiers, vehicle.ID)
	}
	return identifiers
}

// preparedRentalIDs names the rentals a restoration puts back, which are the rows it keeps: every
// other rental on a scenario vehicle is removed, because the scenario is what put it there.
func preparedRentalIDs(vehicles []Vehicle) []string {
	var identifiers []string
	for _, vehicle := range vehicles {
		if vehicle.PreparedRentalID != "" {
			identifiers = append(identifiers, vehicle.PreparedRentalID)
		}
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

func scenarioAccounts(
	ctx context.Context,
	users *auth.UserStore,
	vehicles []Vehicle,
) (map[string]uuid.UUID, error) {
	accounts := make(map[string]uuid.UUID)
	for _, address := range scenarioAddressesOf(vehicles) {
		email, err := auth.ParseEmail(address)
		if err != nil {
			return nil, err
		}
		user, _, err := users.ByEmail(ctx, email)
		if err != nil {
			return nil, fmt.Errorf("scenario account %s is missing; seed first: %w", address, err)
		}
		accounts[address] = user.ID
	}
	return accounts, nil
}

func scenarioUserIDs(vehicles []Vehicle, accounts map[string]uuid.UUID) []uuid.UUID {
	identifiers := make([]uuid.UUID, 0, len(accounts))
	for _, address := range scenarioAddressesOf(vehicles) {
		identifiers = append(identifiers, accounts[address])
	}
	return identifiers
}
