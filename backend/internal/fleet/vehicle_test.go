package fleet_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

var observedAt = time.Date(2026, time.September, 13, 7, 15, 30, 0, time.UTC)

// source builds an inventory of the given percentage of a round capacity, so a case reads as the
// share it is about rather than as a pair of quantities.
func source(kind fleet.SourceKind, percent float64) fleet.EnergySource {
	const capacity = fleet.Amount(100 * fleet.AmountScale)
	return fleet.EnergySource{
		Kind:      kind,
		Remaining: fleet.Amount(percent * fleet.AmountScale),
		Capacity:  capacity,
	}
}

// vehicle builds an otherwise faultless vehicle, so a case changes only the fact it is about.
func vehicle(powertrain fleet.PowertrainType, sources ...fleet.EnergySource) fleet.Vehicle {
	return fleet.Vehicle{
		ID:             "01994342-6ba7-7000-8000-000000000001",
		Model:          "Демо",
		PowertrainType: powertrain,
		Version:        1,
		Connected:      true,
		Telemetry:      fleet.Telemetry{ConfirmedAt: observedAt},
		ServiceZoneID:  "01994342-6ba7-7000-8000-000200000001",
		Sources:        sources,
		HeldBy:         stage.NotHeld,
	}
}

func TestStartThresholdIsMetBySingleSourcesOnly(t *testing.T) {
	for _, tc := range []struct {
		name    string
		vehicle fleet.Vehicle
		status  fleet.Status
	}{
		{
			name:    "exactly the threshold is enough",
			vehicle: vehicle(fleet.PowertrainElectric, source(fleet.SourceBattery, 20)),
			status:  fleet.Available,
		},
		{
			name:    "just under the threshold is not",
			vehicle: vehicle(fleet.PowertrainElectric, source(fleet.SourceBattery, 19.9999)),
			status:  fleet.Unavailable,
		},
		{
			name: "two sources under the threshold are not added together",
			vehicle: vehicle(fleet.PowertrainHybrid,
				source(fleet.SourceBattery, 19),
				source(fleet.SourceGasoline, 19),
			),
			status: fleet.Unavailable,
		},
		{
			name: "an empty battery does not hide a sufficient petrol reserve",
			vehicle: vehicle(fleet.PowertrainHybrid,
				source(fleet.SourceBattery, 0),
				source(fleet.SourceGasoline, 30),
			),
			status: fleet.Available,
		},
		{
			name: "a source the powertrain does not carry cannot start it",
			vehicle: vehicle(fleet.PowertrainElectric,
				source(fleet.SourceBattery, 5),
				source(fleet.SourceDiesel, 90),
			),
			status: fleet.Unavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if state := tc.vehicle.StateAt(observedAt); state.Status != tc.status {
				t.Fatalf("status = %q, want %q (reasons %v)", state.Status, tc.status, state.UnavailableReasons)
			}
		})
	}
}

func TestTelemetryIsFreshUpToAndIncludingTheLimit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		age       time.Duration
		connected bool
		freshness fleet.Freshness
	}{
		{"exactly the limit", fleet.MaxTelemetryAge, true, fleet.FreshTelemetry},
		{"one microsecond past the limit", fleet.MaxTelemetryAge + time.Microsecond, true, fleet.StaleTelemetry},
		{"no link reports offline however recent", 0, false, fleet.OfflineTelemetry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			under := vehicle(fleet.PowertrainElectric, source(fleet.SourceBattery, 90))
			under.Connected = tc.connected
			under.Telemetry.ConfirmedAt = observedAt.Add(-tc.age)
			if freshness := under.TelemetryFreshnessAt(observedAt); freshness != tc.freshness {
				t.Fatalf("freshness = %q, want %q", freshness, tc.freshness)
			}
		})
	}
}

func TestALiveRentalDecidesOccupancyWhateverTheVehicleCondition(t *testing.T) {
	for _, tc := range []struct {
		stage    stage.Stage
		status   fleet.Status
		rideMode fleet.RideMode
	}{
		{stage.Reserved, fleet.Reserved, ""},
		{stage.Active, fleet.InTrip, fleet.Driving},
		{stage.Paused, fleet.InTrip, fleet.Paused},
	} {
		t.Run(string(tc.stage), func(t *testing.T) {
			// Everything that would otherwise make this vehicle unavailable is true at once.
			under := vehicle(fleet.PowertrainElectric, source(fleet.SourceBattery, 1))
			under.Connected = false
			under.ServiceZoneID = ""
			under.HeldBy = tc.stage

			state := under.StateAt(observedAt)
			if state.Status != tc.status || state.RideMode != tc.rideMode {
				t.Fatalf("state = %+v, want %q %q", state, tc.status, tc.rideMode)
			}
			if len(state.UnavailableReasons) != 0 {
				t.Fatalf("an occupied vehicle reported %v", state.UnavailableReasons)
			}
		})
	}
}

func TestEverySimultaneousReasonIsReported(t *testing.T) {
	under := vehicle(fleet.PowertrainGas,
		source(fleet.SourceGasoline, 5),
		source(fleet.SourceLPG, 5),
	)
	under.Connected = false
	under.ServiceZoneID = ""

	state := under.StateAt(observedAt)
	want := []fleet.UnavailableReason{
		fleet.InsufficientEnergy,
		fleet.TechnicalUnavailable,
		fleet.OutsideServiceZone,
	}
	if state.Status != fleet.Unavailable || len(state.UnavailableReasons) != len(want) {
		t.Fatalf("state = %+v, want %v", state, want)
	}
	for index, reason := range want {
		if state.UnavailableReasons[index] != reason {
			t.Fatalf("reasons = %v, want %v", state.UnavailableReasons, want)
		}
	}
}

// faults are the conditions a case can put on an otherwise faultless vehicle, one bit each. Every
// combination of them is built below, because the two questions a vehicle is asked — is it free, and
// may a rental begin on it — read the same conditions and must answer them the same way.
var faults = []struct {
	name    string
	inflict func(*fleet.Vehicle)
}{
	{"exhausted", func(under *fleet.Vehicle) {
		under.Sources = []fleet.EnergySource{source(fleet.SourceBattery, 1)}
	}},
	{"out of service", func(under *fleet.Vehicle) { under.ServiceRequired = true }},
	{"stale telemetry", func(under *fleet.Vehicle) {
		under.Telemetry.ConfirmedAt = observedAt.Add(-fleet.MaxTelemetryAge - time.Second)
	}},
	{"offline", func(under *fleet.Vehicle) { under.Connected = false }},
	{"outside the service zone", func(under *fleet.Vehicle) { under.ServiceZoneID = "" }},
}

// A start and a free rental are two readings of one set of conditions, so this holds them to each
// other over every combination of the conditions rather than over a chosen few: the reasons a start
// is refused must be exactly the reasons the catalog publishes for a start to be refused by.
func TestAStartIsRefusedByTheReasonsTheCatalogPublishes(t *testing.T) {
	for combination := range 1 << len(faults) {
		under := faultyVehicle(combination)
		t.Run(brokenConditions(combination), func(t *testing.T) {
			published := under.StateAt(observedAt)
			if got := under.StartRefusalReasons(); !slices.Equal(got, startRefusals(published.UnavailableReasons)) {
				t.Errorf("a start is refused with %v, while the catalog publishes %v",
					got, published.UnavailableReasons)
			}
		})
	}
}

// The two reasons a start is refused with come in the catalog's own order, so the energy is named
// before the servicing however the conditions happen to be declared together.
func TestAStartNamesTheEnergyBeforeTheServicing(t *testing.T) {
	under := faultyVehicle(1<<len(faults) - 1)
	want := []fleet.UnavailableReason{fleet.InsufficientEnergy, fleet.ServiceRequired}
	if got := under.StartRefusalReasons(); !slices.Equal(got, want) {
		t.Fatalf("a start is refused with %v, want %v", got, want)
	}
}

// faultyVehicle is an otherwise faultless vehicle carrying the conditions of one combination: bit n
// of the combination is fault n.
func faultyVehicle(combination int) fleet.Vehicle {
	under := vehicle(fleet.PowertrainElectric, source(fleet.SourceBattery, 90))
	for index, fault := range faults {
		if combination&(1<<index) != 0 {
			fault.inflict(&under)
		}
	}
	return under
}

// brokenConditions spells a combination as the conditions it carries, so a failing case says which
// vehicle it was about.
func brokenConditions(combination int) string {
	var broken []string
	for index, fault := range faults {
		if combination&(1<<index) != 0 {
			broken = append(broken, fault.name)
		}
	}
	if len(broken) == 0 {
		return "a faultless vehicle"
	}
	return strings.Join(broken, " and ")
}

// startRefusals is the domain rule on its own: a rental is refused on the reserves and on a vehicle
// taken out of service, and on nothing else. Stating it here rather than reading it out of the
// declaration is what makes this a check of the declaration rather than a second copy of it.
func startRefusals(reasons []fleet.UnavailableReason) []fleet.UnavailableReason {
	var refusing []fleet.UnavailableReason
	for _, reason := range reasons {
		if reason == fleet.InsufficientEnergy || reason == fleet.ServiceRequired {
			refusing = append(refusing, reason)
		}
	}
	return refusing
}

func TestSourceCapabilitySeparatesStartingFromContinuing(t *testing.T) {
	under := vehicle(fleet.PowertrainElectric, source(fleet.SourceBattery, 1))
	battery := under.Sources[0]
	if under.CanStart(battery) {
		t.Fatal("a reserve below the threshold started a rental")
	}
	if !battery.CanContinue() {
		t.Fatal("a positive reserve could not continue a rental")
	}
}

func TestAmountRendersTheContractDecimal(t *testing.T) {
	for _, tc := range []struct {
		amount  fleet.Amount
		decimal string
	}{
		{0, "0"},
		{fleet.AmountScale, "1"},
		{1, "0.000001"},
		{1_200_125_000, "1200.125"},
		{2_000 * fleet.AmountScale, "2000"},
	} {
		t.Run(tc.decimal, func(t *testing.T) {
			if rendered := tc.amount.Decimal(); rendered != tc.decimal {
				t.Fatalf("decimal = %q, want %q", rendered, tc.decimal)
			}
		})
	}
}
