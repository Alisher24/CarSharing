package rentals

import (
	"slices"
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

var reservationMoment = time.Date(2026, time.September, 13, 7, 15, 30, 0, time.UTC)

// reservableVehicleAt builds an otherwise faultless free vehicle, so a case changes only the fact it
// is about.
func reservableVehicleAt(sources ...fleet.EnergySource) fleet.Vehicle {
	return fleet.Vehicle{
		ID:             "01994342-6ba7-7000-8000-000000000001",
		PowertrainType: fleet.PowertrainElectric,
		Connected:      true,
		Telemetry:      fleet.Telemetry{ConfirmedAt: reservationMoment},
		ServiceZoneID:  "01994342-6ba7-7000-8000-000200000001",
		Sources:        sources,
		HeldBy:         stage.NotHeld,
	}
}

func batteryAt(percent float64) fleet.EnergySource {
	const capacity = fleet.Amount(100 * fleet.AmountScale)
	return fleet.EnergySource{
		Kind:      fleet.SourceBattery,
		Capacity:  capacity,
		Remaining: fleet.Amount(percent * fleet.AmountScale),
	}
}

// A vehicle may be reserved when it is free and the catalog publishes it as available.
func TestVehicleRefusalAllowsAnAvailableVehicle(t *testing.T) {
	vehicle := reservableVehicleAt(batteryAt(90))
	if refusal := vehicleRefusal(vehicle, reservationMoment); refusal != nil {
		t.Fatalf("an available vehicle was refused with %s", refusal.Kind)
	}
}

// A vehicle somebody holds is refused before its own condition is judged: how full it is says nothing
// about whether it is free, and the refusal names no reason the catalog never published for it.
func TestVehicleRefusalPrefersTheRentalThatHoldsIt(t *testing.T) {
	for _, held := range []stage.Stage{stage.Reserved, stage.Active, stage.Paused} {
		vehicle := reservableVehicleAt(batteryAt(90))
		vehicle.HeldBy = held
		refusal := vehicleRefusal(vehicle, reservationMoment)
		if refusal == nil {
			t.Fatalf("a %s vehicle was offered for reservation", held)
		}
		if refusal.Kind != VehicleUnavailable {
			t.Fatalf("a %s vehicle was refused with %s, want %s", held, refusal.Kind, VehicleUnavailable)
		}
		if len(refusal.UnavailableReasons) != 0 {
			t.Fatalf("a %s vehicle was refused with the reasons %v", held, refusal.UnavailableReasons)
		}
	}
}

// A free vehicle the catalog does not publish as available is refused, and the refusal carries every
// reason the catalog stated, so a client is told what to fix rather than only that it cannot book.
func TestVehicleRefusalCarriesThePublishedReasons(t *testing.T) {
	exhausted := reservableVehicleAt(batteryAt(19))
	refusal := vehicleRefusal(exhausted, reservationMoment)
	if refusal == nil || refusal.Kind != VehicleUnavailable {
		t.Fatalf("an exhausted vehicle was refused with %v, want %s", refusal, VehicleUnavailable)
	}
	want := exhausted.StateAt(reservationMoment).UnavailableReasons
	if !slices.Equal(refusal.UnavailableReasons, want) {
		t.Fatalf("the refusal names %v, while the catalog publishes %v", refusal.UnavailableReasons, want)
	}
	if len(want) == 0 {
		t.Fatal("an exhausted vehicle is published as available by the catalog")
	}
}
