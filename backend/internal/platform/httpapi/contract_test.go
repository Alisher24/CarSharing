package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	mailstubapi "github.com/Alisher24/CarSharing/backend/internal/contracts/mailstubapi"
	publicapi "github.com/Alisher24/CarSharing/backend/internal/contracts/publicapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
)

// The isolated router only returns a fixture. It shares HTTP validation/error adapters
// with production and does not implement or register any domain commands there.
func contractRouter(t *testing.T, spec *openapi3.T) http.Handler {
	t.Helper()
	return boundary(spec, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	}), openapi3filter.NoopAuthenticationFunc)
}

func TestCommandAndPaginationRequestBoundaries(t *testing.T) {
	spec, err := publicapi.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	handler := contractRouter(t, spec)
	const id = "01994342-6ba7-7000-8000-000000000001"
	const key = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		name, method, path, body, key, code string
		status                              int
	}{
		{"reserve", "POST", "/api/v1/reservations", `{"vehicle_id":"` + id + `"}`, key, "", 204},
		{"missing key", "POST", "/api/v1/reservations", `{"vehicle_id":"` + id + `"}`, "", "IDEMPOTENCY_KEY_REQUIRED", 400},
		{"wrong key version", "POST", "/api/v1/reservations", `{"vehicle_id":"` + id + `"}`, id, "IDEMPOTENCY_KEY_INVALID", 400},
		{"no command body", "POST", "/api/v1/rides/" + id + "/pause", "", key, "", 204},
		{"empty object is a body", "POST", "/api/v1/rides/" + id + "/pause", "{}", key, "VALIDATION_FAILED", 422},
		{"resource UUID version", "GET", "/api/v1/vehicles/" + key, "", "", "VALIDATION_FAILED", 422},
		{"first page", "GET", "/api/v1/me/rides?limit=1", "", "", "", 204},
		{"maximum page", "GET", "/api/v1/me/rides?limit=100", "", "", "", 204},
		{"zero limit", "GET", "/api/v1/me/rides?limit=0", "", "", "VALIDATION_FAILED", 422},
		{"excessive limit", "GET", "/api/v1/me/rides?limit=101", "", "", "VALIDATION_FAILED", 422},
		{"bad cursor", "GET", "/api/v1/me/rides?cursor=a%3Db", "", "", "INVALID_CURSOR", 400},
		{"unicode password", "POST", "/api/v1/auth/register", `{"email":"user@example.test","password":"абвгдежзийкл"}`, "", "", 204},
		{"unicode whitespace", "POST", "/api/v1/auth/register", `{"email":"user@example.test","password":"Example\u00a0Password42"}`, "", "VALIDATION_FAILED", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", "http://127.0.0.1:8080")
			r.Header.Set("X-CSRF-Token", "example-csrf-value")
			if tc.key != "" {
				r.Header.Set("Idempotency-Key", tc.key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || tc.code != "" && !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestDemoPositionRejectsLatitudeOutsideWGS84(t *testing.T) {
	spec, err := internalapi.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/internal/v1/demo/actions", strings.NewReader(`{"action_id":"11111111-1111-4111-8111-111111111111","action":"set_position","vehicle_id":"01994342-6ba7-7000-8000-000000000001","position":{"type":"Point","coordinates":[74.6,91]}}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	contractRouter(t, spec).ServeHTTP(w, r)
	if w.Code != 422 || !strings.Contains(w.Body.String(), `/position/coordinates/1`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestInternalContractsAuthenticateBeforePayloadAndUseLargerBodyLimit(t *testing.T) {
	for name, load := range map[string]func() (*openapi3.T, error){"simulation": internalapi.GetSwagger, "mailstub": mailstubapi.GetSwagger} {
		t.Run(name, func(t *testing.T) {
			spec, err := load()
			if err != nil {
				t.Fatal(err)
			}
			auth := func(_ context.Context, input *openapi3filter.AuthenticationInput) error {
				if input.RequestValidationInput.Request.Header.Get("Authorization") != "Bearer example-test-credential" {
					return fmt.Errorf("invalid test credential")
				}
				return nil
			}
			handler := boundary(spec, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), auth)
			path := "/internal/v1/simulation/tick"
			if name == "mailstub" {
				path = "/internal/v1/messages"
			}
			for _, tc := range []struct {
				name, body, token string
				status            int
				code              string
			}{
				{"missing credentials before malformed JSON", "{", "", 401, "INTERNAL_AUTHENTICATION_REQUIRED"},
				{"invalid credentials before malformed JSON", "{", "Bearer wrong", 401, "INTERNAL_AUTHENTICATION_REQUIRED"},
				{"malformed authenticated JSON", "{", "Bearer example-test-credential", 400, "MALFORMED_JSON"},
				{"more than public body limit", `{"unexpected":"` + strings.Repeat("x", 65536) + `"}`, "Bearer example-test-credential", 422, "VALIDATION_FAILED"},
				{"internal limit", strings.Repeat("x", 262145), "Bearer example-test-credential", 413, "BODY_TOO_LARGE"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := httptest.NewRequest("POST", path, strings.NewReader(tc.body))
					r.Header.Set("Content-Type", "application/json")
					r.Header.Set("Authorization", tc.token)
					r.Header.Set("Delivery-Key", "invoice:01994342-6ba7-7000-8000-000000000001:issued")
					w := httptest.NewRecorder()
					handler.ServeHTTP(w, r)
					if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
						t.Fatalf("%d %s", w.Code, w.Body.String())
					}
				})
			}
		})
	}
}

func TestSessionContractReportsRequestErrors(t *testing.T) {
	spec, err := publicapi.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	handler := contractRouter(t, spec)
	for _, tc := range []struct {
		name, body, media, requestID, code string
		status                             int
	}{
		{"malformed JSON", `{"email":`, "application/json", "", "MALFORMED_JSON", 400},
		{"unknown field", `{"email":"user@example.test","password":"ExamplePassword42","admin":true}`, "application/json", "", "VALIDATION_FAILED", 422},
		{"missing fields", `{}`, "application/json", "", "VALIDATION_FAILED", 422},
		{"absent body", ``, "application/json", "", "VALIDATION_FAILED", 422},
		{"invalid header", `{"email":"user@example.test","password":"ExamplePassword42"}`, "application/json", "has a space", "INVALID_HEADER", 400},
		{"media type", `{}`, "text/plain", "", "UNSUPPORTED_MEDIA_TYPE", 415},
		{"body limit", strings.Repeat("x", 65537), "application/json", "", "BODY_TOO_LARGE", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			r.Header.Set("Origin", "http://127.0.0.1:8080")
			if tc.requestID != "" {
				r.Header.Set("X-Request-ID", tc.requestID)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			var body struct {
				Code      string `json:"code"`
				RequestID string `json:"request_id"`
				Details   struct {
					Violations []map[string]any `json:"violations"`
				} `json:"details"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body.Code != tc.code {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
			if body.RequestID == "" || body.RequestID != w.Header().Get("X-Request-ID") {
				t.Fatal("request ID mismatch")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("response can be cached")
			}
			if tc.name == "missing fields" {
				v := body.Details.Violations
				if len(v) != 2 || v[0]["pointer"] != "/email" || v[1]["pointer"] != "/password" {
					t.Fatalf("missing deterministic violations: %s", w.Body.String())
				}
			}
		})
	}
}

func TestPlannedRoutesRemainAbsentFromProduction(t *testing.T) {
	spec, err := publicapi.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	handler := Router(func(context.Context) (Status, error) { return Status{}, nil })
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Extensions["x-implementation-status"] != "planned" {
				continue
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(method, strings.ReplaceAll(path, "{id}", "01994342-6ba7-7000-8000-000000000001"), nil))
			if w.Code != 404 || !strings.Contains(w.Body.String(), `"code":"RESOURCE_NOT_FOUND"`) {
				t.Errorf("%s %s: %d %s", method, path, w.Code, w.Body.String())
			}
		}
	}
}
