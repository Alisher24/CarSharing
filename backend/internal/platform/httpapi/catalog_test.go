package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// The catalog resources a visitor may read without an account.
const (
	vehiclesPath = "/api/v1/vehicles"
	zonesPath    = "/api/v1/zones"
	tariffsPath  = "/api/v1/tariffs"
)

func TestCatalogResourcesAnswerAnonymousReaders(t *testing.T) {
	handler := testRouter(t, servedapi.ReadyStatus{})
	for _, path := range []string{vehiclesPath, zonesPath, tariffsPath} {
		t.Run(path, func(t *testing.T) {
			response := read(handler, path)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			assertMatchesContract(t, path, response)
		})
	}
}

// A catalog response describes vehicles and prices. Nothing in it may name who holds a rental,
// where they are going or what they owe, so the words a rental is described with must be absent.
func TestCatalogPublishesNoRentalOfAnyone(t *testing.T) {
	body := read(testRouter(t, servedapi.ReadyStatus{}), vehiclesPath).Body.String()
	for _, disclosure := range []string{"user", "email", "rental", "invoice", "route", "history"} {
		if strings.Contains(strings.ToLower(body), disclosure) {
			t.Errorf("the catalog published %q: %s", disclosure, body)
		}
	}
}

func TestEachCatalogResourceFailsOnItsOwn(t *testing.T) {
	working := fixedCatalog{snapshot: demoSnapshot(), zones: demoZones(), tariffs: demoTariffs()}
	failing := fixedCatalog{fails: true}
	for _, tc := range []struct {
		name    string
		catalog Catalog
		broken  string
	}{
		{
			name:    "vehicles",
			catalog: Catalog{Vehicles: failing, Zones: zoneReader{working}, Tariffs: tariffReader{working}},
			broken:  vehiclesPath,
		},
		{
			name:    "zones",
			catalog: Catalog{Vehicles: working, Zones: zoneReader{failing}, Tariffs: tariffReader{working}},
			broken:  zonesPath,
		},
		{
			name:    "tariffs",
			catalog: Catalog{Vehicles: working, Zones: zoneReader{working}, Tariffs: tariffReader{failing}},
			broken:  tariffsPath,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewAnonymousRouter(readyProbe, tc.catalog)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{vehiclesPath, zonesPath, tariffsPath} {
				response := read(handler, path)
				wanted := http.StatusOK
				if path == tc.broken {
					wanted = http.StatusServiceUnavailable
				}
				if response.Code != wanted {
					t.Errorf("%s answered %d, want %d", path, response.Code, wanted)
				}
			}
		})
	}
}

func TestOneVehicleIsReadableAndAnUnknownOneIsNot(t *testing.T) {
	handler := testRouter(t, servedapi.ReadyStatus{})
	known := demoSnapshot().Vehicles[0].ID

	found := read(handler, vehiclesPath+"/"+known)
	if found.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", found.Code, found.Body.String())
	}
	assertMatchesContract(t, vehiclesPath+"/{id}", found)

	missing := read(handler, vehiclesPath+"/"+resourceID)
	if missing.Code != http.StatusNotFound || !hasErrorCode(missing, "RESOURCE_NOT_FOUND") {
		t.Fatalf("status = %d: %s", missing.Code, missing.Body.String())
	}
}

// Every prepared state must reach the wire in the shape the contract gives it: a reservation with
// no ride mode, a trip with one, and an exhausted vehicle with the reasons it is unavailable.
func TestEveryPublishedStateKeepsItsOwnShape(t *testing.T) {
	response := read(testRouter(t, servedapi.ReadyStatus{}), vehiclesPath)
	var collection struct {
		Items []struct {
			Status             string   `json:"status"`
			RideMode           string   `json:"ride_mode"`
			UnavailableReasons []string `json:"unavailable_reasons"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &collection); err != nil {
		t.Fatal(err)
	}

	shapes := map[string]bool{}
	for _, item := range collection.Items {
		shapes[item.Status] = true
		if (item.RideMode != "") != (item.Status == "in_trip") {
			t.Errorf("%q carries ride mode %q", item.Status, item.RideMode)
		}
		if (len(item.UnavailableReasons) > 0) != (item.Status == "unavailable") {
			t.Errorf("%q carries reasons %v", item.Status, item.UnavailableReasons)
		}
	}
	for _, wanted := range []string{"available", "reserved", "in_trip", "unavailable"} {
		if !shapes[wanted] {
			t.Errorf("the fixture published no %q vehicle", wanted)
		}
	}
}

func readyProbe(context.Context) (servedapi.ReadyStatus, error) {
	return servedapi.ReadyStatus{}, nil
}

func read(handler http.Handler, path string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

// assertMatchesContract validates a served response against the contract it is declared by, so an
// answer that merely looks right is still held to the schema, the decimal formats and the
// discriminated shapes the contract states.
func assertMatchesContract(t *testing.T, path string, recorded *httptest.ResponseRecorder) {
	t.Helper()
	spec, err := servedapi.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	routes, err := gorillamux.NewRouter(spec)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, strings.ReplaceAll(path, "{id}", resourceID), nil)
	route, pathParams, err := routes.FindRoute(request)
	if err != nil {
		t.Fatal(err)
	}
	validation := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    request,
			PathParams: pathParams,
			Route:      route,
			Options:    &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		},
		Status: recorded.Code,
		Header: recorded.Header(),
		Body:   http.NoBody,
	}
	validation.SetBodyBytes(recorded.Body.Bytes())
	if err = openapi3filter.ValidateResponse(context.Background(), validation); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, recorded.Body.String())
	}
}
