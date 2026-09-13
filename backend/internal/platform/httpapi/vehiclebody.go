package httpapi

import (
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// vehicleBody renders one vehicle in the shape its state selects. The contract publishes a
// different object for each state — a trip carries its mode, an unavailable vehicle its reasons —
// so the state decides the branch rather than a field being left empty on a shared one.
func vehicleBody(vehicle fleet.Vehicle, state fleet.State, freshness fleet.Freshness) (servedapi.Vehicle, error) {
	var body servedapi.Vehicle
	switch state.Status {
	case fleet.Available:
		return body, body.FromAvailableVehicle(availableVehicle(vehicle, freshness))
	case fleet.Reserved:
		return body, body.FromReservedVehicle(reservedVehicle(vehicle, freshness))
	case fleet.InTrip:
		return body, body.FromInTripVehicle(inTripVehicle(vehicle, state, freshness))
	default:
		return body, body.FromUnavailableVehicle(unavailableVehicle(vehicle, state, freshness))
	}
}

func availableVehicle(vehicle fleet.Vehicle, freshness fleet.Freshness) servedapi.AvailableVehicle {
	return servedapi.AvailableVehicle{
		Id:              vehicle.ID,
		Model:           vehicle.Model,
		PowertrainType:  servedapi.PowertrainType(vehicle.PowertrainType),
		Position:        point(vehicle.Telemetry.Position),
		TelemetryAt:     formatTimestamp(vehicle.Telemetry.ConfirmedAt),
		TelemetryStatus: servedapi.AvailableVehicleTelemetryStatus(freshness),
		EnergySources:   energySources(vehicle),
		Version:         exactInteger(vehicle.Version),
		Status:          servedapi.AvailableVehicleStatus(fleet.Available),
	}
}

func reservedVehicle(vehicle fleet.Vehicle, freshness fleet.Freshness) servedapi.ReservedVehicle {
	return servedapi.ReservedVehicle{
		Id:              vehicle.ID,
		Model:           vehicle.Model,
		PowertrainType:  servedapi.PowertrainType(vehicle.PowertrainType),
		Position:        point(vehicle.Telemetry.Position),
		TelemetryAt:     formatTimestamp(vehicle.Telemetry.ConfirmedAt),
		TelemetryStatus: servedapi.ReservedVehicleTelemetryStatus(freshness),
		EnergySources:   energySources(vehicle),
		Version:         exactInteger(vehicle.Version),
		Status:          servedapi.ReservedVehicleStatus(fleet.Reserved),
	}
}

func inTripVehicle(
	vehicle fleet.Vehicle, state fleet.State, freshness fleet.Freshness,
) servedapi.InTripVehicle {
	return servedapi.InTripVehicle{
		Id:              vehicle.ID,
		Model:           vehicle.Model,
		PowertrainType:  servedapi.PowertrainType(vehicle.PowertrainType),
		Position:        point(vehicle.Telemetry.Position),
		TelemetryAt:     formatTimestamp(vehicle.Telemetry.ConfirmedAt),
		TelemetryStatus: servedapi.InTripVehicleTelemetryStatus(freshness),
		EnergySources:   energySources(vehicle),
		Version:         exactInteger(vehicle.Version),
		Status:          servedapi.InTripVehicleStatus(fleet.InTrip),
		RideMode:        servedapi.InTripVehicleRideMode(state.RideMode),
	}
}

func unavailableVehicle(
	vehicle fleet.Vehicle, state fleet.State, freshness fleet.Freshness,
) servedapi.UnavailableVehicle {
	reasons := make([]servedapi.UnavailableReason, 0, len(state.UnavailableReasons))
	for _, reason := range state.UnavailableReasons {
		reasons = append(reasons, servedapi.UnavailableReason(reason))
	}
	return servedapi.UnavailableVehicle{
		Id:                 vehicle.ID,
		Model:              vehicle.Model,
		PowertrainType:     servedapi.PowertrainType(vehicle.PowertrainType),
		Position:           point(vehicle.Telemetry.Position),
		TelemetryAt:        formatTimestamp(vehicle.Telemetry.ConfirmedAt),
		TelemetryStatus:    servedapi.UnavailableVehicleTelemetryStatus(freshness),
		EnergySources:      energySources(vehicle),
		Version:            exactInteger(vehicle.Version),
		Status:             servedapi.UnavailableVehicleStatus(fleet.Unavailable),
		UnavailableReasons: reasons,
	}
}

// energySources renders every inventory separately. Nothing here adds two of them together: the
// share of each is published on its own, and so is what each one alone permits.
func energySources(vehicle fleet.Vehicle) []servedapi.EnergySource {
	published := make([]servedapi.EnergySource, 0, len(vehicle.Sources))
	for _, source := range vehicle.Sources {
		unit, _ := fleet.UnitOf(source.Kind)
		published = append(published, servedapi.EnergySource{
			Kind:                 servedapi.SourceKind(source.Kind),
			Remaining:            source.Remaining.Decimal(),
			Capacity:             source.Capacity.Decimal(),
			Unit:                 servedapi.EnergySourceUnit(unit),
			RemainingBasisPoints: source.RemainingBasisPoints(),
			CanStart:             vehicle.CanStart(source),
			CanContinue:          source.CanContinue(),
		})
	}
	return published
}

func point(position fleet.Position) servedapi.Point {
	return servedapi.Point{
		Type:        servedapi.PointType(geoJSONPointType),
		Coordinates: servedapi.Position{position.Longitude, position.Latitude},
	}
}
