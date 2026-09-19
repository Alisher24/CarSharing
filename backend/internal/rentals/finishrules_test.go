package rentals

import (
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

// The zone the demonstrations are made in and the area beside it, as identifiers: what a finish asks of
// a position is whether the area the reservation was made in still covers it, not which area that is.
const (
	theZoneTheRideWasMadeIn = "01994342-6ba7-7000-8000-000000000001"
	theZoneBesideIt         = "01994342-6ba7-7000-8000-000000000002"
)

// A ride is ended wherever it stands while the area of its reservation covers the position the vehicle
// confirmed. Every row of the boundary table of the product design that concerns the area is stated
// here as the zone the position falls in, which is what the coverage test of the database answers.
func TestAFinishIsAllowedWhereTheAreaOfTheRideCoversTheVehicle(t *testing.T) {
	for _, placed := range []struct {
		name    string
		covered string
		refused bool
	}{
		{name: "inside the area", covered: theZoneTheRideWasMadeIn},
		{name: "on the edge of the area", covered: theZoneTheRideWasMadeIn},
		{name: "on a vertex of the area", covered: theZoneTheRideWasMadeIn},
		{name: "outside every area", covered: "", refused: true},
		{name: "in the area beside the one the ride was made in", covered: theZoneBesideIt, refused: true},
	} {
		t.Run(placed.name, func(t *testing.T) {
			refusal := finishLandingRefusal(
				vehicleConfirmedAt(placed.covered, freshAt), theZoneTheRideWasMadeIn, freshAt)
			if placed.refused {
				if refusal == nil {
					t.Fatal("a finish outside the area of the ride was allowed")
				}
				if refusal.Kind != OutsideServiceZone {
					t.Fatalf("the finish was refused with %q, want %q",
						refusal.Kind, OutsideServiceZone)
				}
				return
			}
			if refusal != nil {
				t.Fatalf("a finish inside the area of the ride was refused with %q", refusal.Kind)
			}
		})
	}
}

// A position is usable to the microsecond the rule states and not one microsecond past it: a reading of
// exactly the limit is fresh and the next one is not, and a vehicle the platform has no link to is
// answered the same way because an unconfirmed reading is not one either.
func TestAFinishIsAllowedOnlyOnAPositionThatCanBeTrusted(t *testing.T) {
	fresh := vehicleConfirmedAt(theZoneTheRideWasMadeIn, freshAt)
	for _, reading := range []struct {
		name    string
		vehicle fleet.Vehicle
		refused bool
	}{
		{name: "confirmed now", vehicle: fresh},
		{
			name: "confirmed exactly the limit ago",
			vehicle: vehicleConfirmedAt(
				theZoneTheRideWasMadeIn, freshAt.Add(-fleet.MaxTelemetryAge)),
		},
		{
			name: "confirmed one microsecond past the limit",
			vehicle: vehicleConfirmedAt(
				theZoneTheRideWasMadeIn, freshAt.Add(-fleet.MaxTelemetryAge-time.Microsecond)),
			refused: true,
		},
		{name: "not reporting", vehicle: disconnectedVehicle(theZoneTheRideWasMadeIn), refused: true},
	} {
		t.Run(reading.name, func(t *testing.T) {
			refusal := finishLandingRefusal(
				reading.vehicle, theZoneTheRideWasMadeIn, freshAt)
			if !reading.refused {
				if refusal != nil {
					t.Fatalf("a usable position was refused with %q", refusal.Kind)
				}
				return
			}
			if refusal == nil {
				t.Fatal("an unusable position was allowed to decide a finish")
			}
			if refusal.Kind != TelemetryStale {
				t.Fatalf("the finish was refused with %q, want %q", refusal.Kind, TelemetryStale)
			}
		})
	}
}

// A finish applies to a ride that has begun and to nothing else. A rental that has ended is not a
// refusal — finishWithin answers the stored ending before this rule is asked — so it is reported as a
// state this command does not apply to, as is a reservation.
func TestAFinishAppliesOnlyToARideThatHasBegun(t *testing.T) {
	for _, asked := range []struct {
		stage   stage.Stage
		applies bool
	}{
		{stage: stage.Active, applies: true},
		{stage: stage.Paused, applies: true},
		{stage: stage.Reserved},
		{stage: stage.Cancelled},
		{stage: stage.Expired},
		{stage: stage.Completed},
	} {
		t.Run(string(asked.stage), func(t *testing.T) {
			refusal := finishRefusal(Rental{Stage: asked.stage})
			if asked.applies {
				if refusal != nil {
					t.Fatalf("a finish of a %s rental was refused with %q", asked.stage, refusal.Kind)
				}
				return
			}
			if refusal == nil {
				t.Fatalf("a finish of a %s rental was allowed", asked.stage)
			}
			if refusal.Kind != InvalidRentalState {
				t.Fatalf("a %s rental was refused with %q, want %q",
					asked.stage, refusal.Kind, InvalidRentalState)
			}
		})
	}
}

// freshAt is the one moment every check above judges a position against, so a reading is stated as an
// age rather than against a clock that moves between rows.
var freshAt = time.Date(2026, time.September, 14, 7, 30, 30, 123456000, time.UTC)

// vehicleConfirmedAt is a vehicle the coverage test of the database placed in one area, with a reading
// confirmed at one moment. A zone the database found no area for is the empty string, which is how a
// position outside every service area is stated.
func vehicleConfirmedAt(zoneID string, confirmedAt time.Time) fleet.Vehicle {
	return fleet.Vehicle{
		ID:            "01994342-6ba7-7000-8000-00000000000a",
		Connected:     true,
		ServiceZoneID: zoneID,
		Telemetry:     fleet.Telemetry{ConfirmedAt: confirmedAt},
	}
}

func disconnectedVehicle(zoneID string) fleet.Vehicle {
	vehicle := vehicleConfirmedAt(zoneID, freshAt)
	vehicle.Connected = false
	return vehicle
}
