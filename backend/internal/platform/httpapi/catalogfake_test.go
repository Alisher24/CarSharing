package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/Alisher24/CarSharing/backend/internal/zones"
)

// errCatalogUnreadable stands for a store that cannot answer, so that a case about a failing
// resource does not need a database to produce one.
var errCatalogUnreadable = errors.New("catalog store did not answer")

// fixedCatalog answers from values a case states, or refuses when it is given no values. It is the
// seam that lets the served router be exercised without a database behind it.
type fixedCatalog struct {
	snapshot fleet.Snapshot
	zones    []zones.Zone
	tariffs  []tariffs.Tariff
	fails    bool
}

func (c fixedCatalog) Snapshot(context.Context) (fleet.Snapshot, error) {
	if c.fails {
		return fleet.Snapshot{}, errCatalogUnreadable
	}
	return c.snapshot, nil
}

func (c fixedCatalog) Vehicle(_ context.Context, id string) (fleet.Snapshot, error) {
	if c.fails {
		return fleet.Snapshot{}, errCatalogUnreadable
	}
	for _, vehicle := range c.snapshot.Vehicles {
		if vehicle.ID == id {
			return fleet.Snapshot{ObservedAt: c.snapshot.ObservedAt, Vehicles: []fleet.Vehicle{vehicle}}, nil
		}
	}
	return fleet.Snapshot{}, fleet.ErrVehicleNotFound
}

func (c fixedCatalog) Current(context.Context) ([]zones.Zone, error) {
	if c.fails {
		return nil, errCatalogUnreadable
	}
	return c.zones, nil
}

// zoneReader and tariffReader narrow one fixture to the reader each operation needs, because both
// resources are read through a method of the same name.
type zoneReader struct{ fixedCatalog }

type tariffReader struct{ fixedCatalog }

func (r tariffReader) Current(context.Context) ([]tariffs.Tariff, error) {
	if r.fails {
		return nil, errCatalogUnreadable
	}
	return r.tariffs, nil
}

func (c fixedCatalog) asCatalog() Catalog {
	return Catalog{
		Vehicles: c,
		Zones:    zoneReader{fixedCatalog: c},
		Tariffs:  tariffReader{fixedCatalog: c},
	}
}

// demoSnapshot is one vehicle of each published state, read at a fixed instant.
func demoSnapshot() fleet.Snapshot {
	observedAt := time.Date(2026, time.September, 13, 7, 15, 30, 0, time.UTC)
	return fleet.Snapshot{
		ObservedAt: observedAt,
		Vehicles: []fleet.Vehicle{
			catalogVehicle(observedAt, "01994342-6ba7-7000-8000-000100000001", 9000, stage.NotHeld),
			catalogVehicle(observedAt, "01994342-6ba7-7000-8000-000100000002", 9000, stage.Reserved),
			catalogVehicle(observedAt, "01994342-6ba7-7000-8000-000100000003", 9000, stage.Paused),
			catalogVehicle(observedAt, "01994342-6ba7-7000-8000-000100000004", 100, stage.NotHeld),
		},
	}
}

func catalogVehicle(
	observedAt time.Time, id string, remainingBasisPoints int64, heldBy stage.Stage,
) fleet.Vehicle {
	const capacity = fleet.Amount(10_000 * fleet.AmountScale)
	return fleet.Vehicle{
		ID:             id,
		Model:          "Демо Электро 1",
		PowertrainType: fleet.PowertrainElectric,
		Version:        1,
		Connected:      true,
		Telemetry: fleet.Telemetry{
			Position:    fleet.Position{Longitude: 74.6, Latitude: 42.87},
			ConfirmedAt: observedAt,
		},
		ServiceZoneID: "01994342-6ba7-7000-8000-000200000001",
		Sources: []fleet.EnergySource{{
			Kind:      fleet.SourceBattery,
			Capacity:  capacity,
			Remaining: fleet.Amount(int64(capacity) * remainingBasisPoints / 10_000),
		}},
		HeldBy: heldBy,
	}
}

func demoZones() []zones.Zone {
	return []zones.Zone{{
		ID:      "01994342-6ba7-7000-8000-000200000001",
		Name:    "Демонстрационная зона Бишкека",
		Version: 1,
		Area: []byte(`{"type":"Polygon","coordinates":` +
			`[[[74.55,42.84],[74.65,42.84],[74.65,42.90],[74.55,42.90],[74.55,42.84]]]}`),
	}}
}

func demoTariffs() []tariffs.Tariff {
	return []tariffs.Tariff{{
		ID:                               "01994342-6ba7-7000-8000-000300000001",
		Currency:                         "KGS",
		BillingPolicy:                    "per_mode_started_minute_v1",
		DrivingRateTyiynPerStartedMinute: 1234,
		PausedRateTyiynPerStartedMinute:  321,
		Version:                          1,
	}}
}

// testRouter is the router over a probe that answers with the given readiness and a catalog that
// answers from fixtures, so a test about which paths are served does not have to build an
// application of its own.
func testRouter(t *testing.T, readiness servedapi.ReadyStatus) http.Handler {
	t.Helper()
	return anonymousRouter(t, fixedCatalog{
		snapshot: demoSnapshot(),
		zones:    demoZones(),
		tariffs:  demoTariffs(),
	}, readiness)
}

func anonymousRouter(t *testing.T, catalog fixedCatalog, readiness servedapi.ReadyStatus) http.Handler {
	t.Helper()
	handler, err := NewAnonymousRouter(func(context.Context) (servedapi.ReadyStatus, error) {
		return readiness, nil
	}, catalog.asCatalog())
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
