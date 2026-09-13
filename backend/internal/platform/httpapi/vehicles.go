package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// geoJSONPointType is the type tag a published coordinate pair carries.
const geoJSONPointType = "Point"

// VehicleReader is the read model the catalog operations answer from. The catalog only reads: a
// status written here would be a second owner of what the rentals module decides.
type VehicleReader interface {
	// Snapshot is every published vehicle together with the instant it was read at.
	Snapshot(ctx context.Context) (fleet.Snapshot, error)

	// Vehicle is one published vehicle, or fleet.ErrVehicleNotFound.
	Vehicle(ctx context.Context, id string) (fleet.Snapshot, error)
}

// vehicles answers the two catalog operations. Both are anonymous, and neither publishes who holds
// a rental, where they are going, or what they have been charged.
type vehicles struct{ reader VehicleReader }

func (v vehicles) GetVehicles(
	ctx context.Context, _ servedapi.GetVehiclesRequestObject,
) (servedapi.GetVehiclesResponseObject, error) {
	snapshot, err := v.reader.Snapshot(ctx)
	if err != nil {
		return servedapi.GetVehicles503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	collection, err := vehicleCollection(snapshot)
	if err != nil {
		return nil, err
	}
	return servedapi.GetVehicles200JSONResponse{Body: collection}, nil
}

func (v vehicles) GetVehicle(
	ctx context.Context, request servedapi.GetVehicleRequestObject,
) (servedapi.GetVehicleResponseObject, error) {
	snapshot, err := v.reader.Vehicle(ctx, request.Id)
	if errors.Is(err, fleet.ErrVehicleNotFound) {
		return servedapi.GetVehicle404JSONResponse{
			Body: apiErrorBody(ctx, codeResourceNotFound, messageResourceNotFound),
		}, nil
	}
	if err != nil {
		return servedapi.GetVehicle503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	body, err := publishedVehicle(snapshot, snapshot.Vehicles[0])
	if err != nil {
		return nil, err
	}
	return servedapi.GetVehicle200JSONResponse{Body: body}, nil
}

func vehicleCollection(snapshot fleet.Snapshot) (servedapi.VehicleCollection, error) {
	collection := servedapi.VehicleCollection{
		ServerTime: formatTimestamp(snapshot.ObservedAt),
		Items:      make([]servedapi.Vehicle, 0, len(snapshot.Vehicles)),
	}
	for _, vehicle := range snapshot.Vehicles {
		body, err := publishedVehicle(snapshot, vehicle)
		if err != nil {
			return servedapi.VehicleCollection{}, err
		}
		collection.Items = append(collection.Items, body)
	}
	return collection, nil
}

// publishedVehicle derives the state at the instant the snapshot was read, so every vehicle in one
// response is judged fresh or stale against the same clock.
func publishedVehicle(snapshot fleet.Snapshot, vehicle fleet.Vehicle) (servedapi.Vehicle, error) {
	state := vehicle.StateAt(snapshot.ObservedAt)
	freshness := vehicle.TelemetryFreshnessAt(snapshot.ObservedAt)
	body, err := vehicleBody(vehicle, state, freshness)
	if err != nil {
		return servedapi.Vehicle{}, fmt.Errorf("vehicle %s cannot be published: %w", vehicle.ID, err)
	}
	return body, nil
}

// exactInteger renders a whole number the way the contract carries one: as a decimal string, so a
// client that parses JSON numbers as doubles cannot lose a digit of it.
func exactInteger(value int64) servedapi.ExactInteger {
	return strconv.FormatInt(value, 10)
}
