package demo_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/demo"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

// declaredFleetSize is the size the demonstration promises: five vehicles of each of the five
// powertrains.
const declaredFleetSize = 25

// observedAt stands for the moment the catalog is read. Every declared vehicle has just confirmed
// its position when the scenario is installed, so freshness is not what these cases are about.
var observedAt = time.Date(2026, time.September, 13, 7, 15, 30, 0, time.UTC)

// published resolves a declared vehicle to the state the catalog would show for it.
func published(vehicle demo.Vehicle) fleet.State {
	return fleet.Vehicle{
		PowertrainType: vehicle.PowertrainType,
		Connected:      vehicle.Connected,
		Telemetry:      fleet.Telemetry{Position: vehicle.Position, ConfirmedAt: observedAt},
		ServiceZoneID:  "01994342-6ba7-7000-8000-000200000001",
		Sources:        vehicle.Sources,
		HeldBy:         vehicle.HeldBy,
	}.StateAt(observedAt)
}

func TestEveryPowertrainShowsEveryPublicState(t *testing.T) {
	declared := demo.Fleet()
	if len(declared) != declaredFleetSize {
		t.Fatalf("fleet has %d vehicles, want %d", len(declared), declaredFleetSize)
	}

	wanted := map[fleet.Status]int{
		fleet.Available:   2,
		fleet.Reserved:    1,
		fleet.InTrip:      1,
		fleet.Unavailable: 1,
	}
	for _, powertrain := range fleet.PowertrainTypes() {
		t.Run(string(powertrain), func(t *testing.T) {
			counted := map[fleet.Status]int{}
			for _, vehicle := range declared {
				if vehicle.PowertrainType == powertrain {
					counted[published(vehicle).Status]++
				}
			}
			for status, count := range wanted {
				if counted[status] != count {
					t.Errorf("%q appears %d times, want %d", status, counted[status], count)
				}
			}
		})
	}
}

func TestPreparedTripsShowBothDrivingAndPausing(t *testing.T) {
	modes := map[fleet.RideMode]int{}
	for _, vehicle := range demo.Fleet() {
		if state := published(vehicle); state.Status == fleet.InTrip {
			modes[state.RideMode]++
		}
	}
	if modes[fleet.Driving] == 0 || modes[fleet.Paused] == 0 {
		t.Fatalf("prepared trips are %v, want both driving and paused", modes)
	}
}

func TestEveryPowertrainKeepsAnExhaustedExample(t *testing.T) {
	for _, powertrain := range fleet.PowertrainTypes() {
		t.Run(string(powertrain), func(t *testing.T) {
			for _, vehicle := range demo.Fleet() {
				if vehicle.PowertrainType != powertrain {
					continue
				}
				state := published(vehicle)
				if state.Status == fleet.Unavailable && hasReason(state, fleet.InsufficientEnergy) {
					return
				}
			}
			t.Fatal("no vehicle of this powertrain stands as an exhausted example")
		})
	}
}

func TestExhaustedExamplesAlsoShowStaleAndOfflineVehicles(t *testing.T) {
	var stale, offline int
	for _, vehicle := range demo.Fleet() {
		switch {
		case !vehicle.Connected:
			offline++
		case !vehicle.Reporting:
			stale++
		}
	}
	if stale == 0 || offline == 0 {
		t.Fatalf("%d vehicles go stale and %d are unlinked, want at least one of each", stale, offline)
	}
}

// A vehicle a person may book by hand must be left alone by the scenario command, and every
// vehicle the command does put back must be one the demonstration itself set up.
func TestRestorationCoversPreparedVehiclesOnly(t *testing.T) {
	for _, vehicle := range demo.Fleet() {
		free := published(vehicle).Status == fleet.Available
		if free == vehicle.Restored() {
			t.Errorf("%s: available %t, restored %t", vehicle.Model, free, vehicle.Restored())
		}
	}
}

// A prepared rental belongs to a service account of its own, because one account may hold only one
// live rental and none of them may be an account a person signs in as.
func TestEveryPreparedRentalHasItsOwnServiceAccount(t *testing.T) {
	accounts := map[string]string{}
	for _, vehicle := range demo.Fleet() {
		if vehicle.HeldBy == stage.NotHeld {
			if vehicle.ScenarioAccount != "" || vehicle.PreparedRentalID != "" {
				t.Errorf("%s has no prepared rental but names one", vehicle.Model)
			}
			continue
		}
		if previous, taken := accounts[vehicle.ScenarioAccount]; taken {
			t.Errorf("%s shares an account with %s", vehicle.Model, previous)
		}
		accounts[vehicle.ScenarioAccount] = vehicle.Model
	}
	for _, manual := range demo.ManualCheckAccounts {
		if held, taken := accounts[manual]; taken {
			t.Errorf("a manual check account holds the rental of %s", held)
		}
	}
}

// A vehicle parked outside the boundary would be published as outside the service zone and would
// stop being the example it was declared to be, so every declared position is checked against the
// area the command actually installs rather than against a copy of its corners. That area is a
// rectangle, so its bounding box is the area itself.
func TestEveryDeclaredVehicleStandsInsideTheServiceZone(t *testing.T) {
	west, south, east, north := installedAreaBounds(t)
	for _, vehicle := range demo.Fleet() {
		position := vehicle.Position
		inside := position.Longitude >= west && position.Longitude <= east &&
			position.Latitude >= south && position.Latitude <= north
		if !inside {
			t.Errorf("%s stands at %v, %v: outside the installed area",
				vehicle.Model, position.Longitude, position.Latitude)
		}
	}
}

func installedAreaBounds(t *testing.T) (west, south, east, north float64) {
	t.Helper()
	var area struct {
		Coordinates [][][2]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal(demo.Zone().Area, &area); err != nil {
		t.Fatalf("the installed area is not readable as GeoJSON: %v", err)
	}
	if len(area.Coordinates) != 1 || len(area.Coordinates[0]) < 3 {
		t.Fatalf("the installed area is %d rings, want one ring of at least three points", len(area.Coordinates))
	}
	ring := area.Coordinates[0]
	west, south, east, north = ring[0][0], ring[0][1], ring[0][0], ring[0][1]
	for _, corner := range ring {
		west, east = min(west, corner[0]), max(east, corner[0])
		south, north = min(south, corner[1]), max(north, corner[1])
	}
	return west, south, east, north
}

func hasReason(state fleet.State, wanted fleet.UnavailableReason) bool {
	for _, reason := range state.UnavailableReasons {
		if reason == wanted {
			return true
		}
	}
	return false
}
