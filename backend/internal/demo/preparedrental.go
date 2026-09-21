package demo

import (
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/google/uuid"
)

func (v Vehicle) preparedRental(owner uuid.UUID, tariffID, zoneID string) rentals.PreparedRental {
	return rentals.PreparedRental{
		ID:        v.PreparedRentalID,
		UserID:    owner,
		VehicleID: v.ID,
		Stage:     v.HeldBy,
		TariffID:  tariffID,
		ZoneID:    zoneID,
	}
}
