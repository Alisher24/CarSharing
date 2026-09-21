package demo

import (
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
)

// RestoreStores are the record owners used to restore existing demonstration data.
type RestoreStores struct {
	Users   *auth.UserStore
	Fleet   *fleet.Store
	Rentals *rentals.Store
	Models  *simulation.Store
}

// NewRestoreStores validates the record owners required by demonstration restoration.
func NewRestoreStores(
	users *auth.UserStore,
	vehicles *fleet.Store,
	rentalRecords *rentals.Store,
	models *simulation.Store,
) (RestoreStores, error) {
	stores := RestoreStores{
		Users:   users,
		Fleet:   vehicles,
		Rentals: rentalRecords,
		Models:  models,
	}
	required := []struct {
		name     string
		supplied bool
	}{
		{"users", users != nil},
		{"fleet", vehicles != nil},
		{"rentals", rentalRecords != nil},
		{"simulation models", models != nil},
	}
	for _, dependency := range required {
		if !dependency.supplied {
			return RestoreStores{}, fmt.Errorf("demonstration record owner %s is missing", dependency.name)
		}
	}
	return stores, nil
}
