package demo

import (
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/Alisher24/CarSharing/backend/internal/zones"
)

// SeedStores are the record owners used to install missing demonstration data.
type SeedStores struct {
	Users   *auth.UserStore
	Fleet   *fleet.Store
	Rentals *rentals.Store
	Tariffs *tariffs.Store
	Zones   *zones.Store
}

// NewSeedStores validates the record owners required by demonstration installation.
func NewSeedStores(
	users *auth.UserStore,
	vehicles *fleet.Store,
	rentalRecords *rentals.Store,
	prices *tariffs.Store,
	serviceAreas *zones.Store,
) (SeedStores, error) {
	stores := SeedStores{
		Users:   users,
		Fleet:   vehicles,
		Rentals: rentalRecords,
		Tariffs: prices,
		Zones:   serviceAreas,
	}
	required := []struct {
		name     string
		supplied bool
	}{
		{"users", users != nil},
		{"fleet", vehicles != nil},
		{"rentals", rentalRecords != nil},
		{"tariffs", prices != nil},
		{"service zones", serviceAreas != nil},
	}
	for _, dependency := range required {
		if !dependency.supplied {
			return SeedStores{}, fmt.Errorf("demonstration record owner %s is missing", dependency.name)
		}
	}
	return stores, nil
}
