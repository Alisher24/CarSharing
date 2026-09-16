package httpapi

import (
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// rideSummaryBody publishes one finished ride of the history: the vehicle it was taken on as it was
// named then, the moments it ran between, why it ended, and the invoice it was charged by.
//
// The summary states no amount. What a ride cost lives in the invoice the account reads beside it,
// and a second copy of a total here would be a number the history could disagree with.
func rideSummaryBody(ride rentals.Ride) (servedapi.RideSummary, error) {
	reason, err := completionBody(ride.Completion, ride.Exhausted)
	if err != nil {
		return servedapi.RideSummary{}, err
	}
	return servedapi.RideSummary{
		Id:          ride.ID,
		Vehicle:     rideVehicleBody(ride.Vehicle),
		StartedAt:   timestamp.Format(ride.StartedAt),
		CompletedAt: timestamp.Format(ride.CompletedAt),
		InvoiceId:   ride.InvoiceID,
		Completion:  reason,
	}, nil
}

// rideVehicleBody publishes the vehicle of a finished ride. It is a historical reference rather than
// a catalog entry: where the vehicle stands and what it holds now say nothing about the ride that is
// over.
func rideVehicleBody(vehicle rentals.RideVehicle) servedapi.VehicleReference {
	return servedapi.VehicleReference{
		Id:             vehicle.ID,
		Model:          vehicle.Model,
		PowertrainType: servedapi.PowertrainType(vehicle.PowertrainType),
	}
}
