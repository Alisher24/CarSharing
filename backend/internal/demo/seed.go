package demo

import (
	"context"
	"errors"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Seed installs whatever part of the demonstration scenario is missing and leaves everything that
// is already there exactly as it stands: a second run adds no row, moves no vehicle, spends no
// reserve and does not push a reservation deadline further out.
func Seed(
	ctx context.Context, pool *pgxpool.Pool, hasher *auth.PasswordHasher, manualPassword string,
) error {
	vehicles := Fleet()
	accounts, err := accountsFor(vehicles, hasher, manualPassword)
	if err != nil {
		return err
	}

	installation := installer{
		store:    store{pool: pool},
		users:    auth.NewUserStore(pool),
		vehicles: vehicles,
	}
	return database.InTransaction(ctx, pool, func(txCtx context.Context) error {
		return installation.install(txCtx, accounts)
	})
}

// installer writes one whole demonstration scenario. It keeps the account identities it created so
// that a prepared rental is attached to the service account that owns it without reading them back.
type installer struct {
	store    store
	users    *auth.UserStore
	vehicles []Vehicle
}

func (i installer) install(ctx context.Context, accounts []account) error {
	zone, tariff := Zone(), Tariff()
	if err := i.store.insertZone(ctx, zone); err != nil {
		return err
	}
	if err := i.store.insertTariff(ctx, tariff); err != nil {
		return err
	}
	if err := i.installVehicles(ctx); err != nil {
		return err
	}
	identities, err := i.installAccounts(ctx, accounts)
	if err != nil {
		return err
	}
	return i.installRentals(ctx, identities, zone.ID, tariff.ID)
}

func (i installer) installVehicles(ctx context.Context) error {
	for _, vehicle := range i.vehicles {
		if err := i.store.insertVehicle(ctx, vehicle); err != nil {
			return err
		}
		if err := i.store.insertEnergySources(ctx, vehicle); err != nil {
			return err
		}
		if err := i.store.insertTelemetry(ctx, vehicle); err != nil {
			return err
		}
	}
	return nil
}

// installAccounts creates the demonstration accounts that are missing and reports the identity of
// each one. An account that already exists keeps the password it has: seeding never resets one.
func (i installer) installAccounts(
	ctx context.Context, accounts []account,
) (map[string]uuid.UUID, error) {
	identities := make(map[string]uuid.UUID, len(accounts))
	for _, demonstration := range accounts {
		user, err := i.users.Create(ctx, demonstration.email, demonstration.passwordHash)
		if errors.Is(err, auth.ErrEmailTaken) {
			user, _, err = i.users.ByEmail(ctx, demonstration.email)
		}
		if err != nil {
			return nil, err
		}
		identities[string(demonstration.email)] = user.ID
	}
	return identities, nil
}

func (i installer) installRentals(
	ctx context.Context, identities map[string]uuid.UUID, zoneID, tariffID string,
) error {
	for _, vehicle := range i.vehicles {
		if vehicle.HeldBy == stage.NotHeld {
			continue
		}
		owner := identities[vehicle.ScenarioAccount]
		if err := i.store.insertRental(ctx, vehicle.preparedRental(owner, tariffID, zoneID)); err != nil {
			return err
		}
	}
	return nil
}
