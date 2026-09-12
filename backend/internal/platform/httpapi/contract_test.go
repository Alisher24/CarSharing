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

const (
	// resourceID is a UUIDv7, which the contract requires for a stored resource; commandID is a
	// UUIDv4, which it requires for a client-chosen idempotency key. Using one where the other
	// belongs is rejected, so the tests below use them to prove each parameter checks its version.
	resourceID = "01994342-6ba7-7000-8000-000000000001"
	commandID  = "11111111-1111-4111-8111-111111111111"

	reservationsPath = "/api/v1/reservations"
	registerPath     = "/api/v1/auth/register"
	myRidesPath      = "/api/v1/me/rides"
	pausePath        = "/api/v1/rides/" + resourceID + "/pause"

	reserveBody              = `{"vehicle_id":"` + resourceID + `"}`
	registerBody             = `{"email":"user@example.test","password":"ExamplePassword42"}`
	registerUnknownFieldBody = `{"email":"user@example.test","password":"ExamplePassword42","admin":true}`
	registerCyrillicBody     = `{"email":"user@example.test","password":"абвгдежзийкл"}`
	registerNbspBody         = `{"email":"user@example.test","password":"Example Password42"}`

	testCredential = "Bearer example-test-credential"
)

// The isolated router only returns a fixture. It shares HTTP validation/error adapters
// with production and does not implement or register any domain commands there.
func contractRouter(t *testing.T, spec *openapi3.T) http.Handler {
	t.Helper()
	return boundary(spec, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", jsonMediaType)
		w.WriteHeader(http.StatusNoContent)
	}), openapi3filter.NoopAuthenticationFunc)
}

func TestCommandAndPaginationRequestBoundaries(t *testing.T) {
	spec, err := publicapi.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	handler := contractRouter(t, spec)
	for _, tc := range []struct {
		name, method, path, body, key, code string
		status                              int
	}{
		{"reserve", "POST", reservationsPath, reserveBody, commandID, "", 204},
		{"missing key", "POST", reservationsPath, reserveBody, "", "IDEMPOTENCY_KEY_REQUIRED", 400},
		{"wrong key version", "POST", reservationsPath, reserveBody, resourceID, "IDEMPOTENCY_KEY_INVALID", 400},
		{"no command body", "POST", pausePath, "", commandID, "", 204},
		{"empty object is a body", "POST", pausePath, "{}", commandID, "VALIDATION_FAILED", 422},
		{"resource UUID version", "GET", "/api/v1/vehicles/" + commandID, "", "", "VALIDATION_FAILED", 422},
		{"first page", "GET", myRidesPath + "?limit=1", "", "", "", 204},
		{"maximum page", "GET", myRidesPath + "?limit=100", "", "", "", 204},
		{"zero limit", "GET", myRidesPath + "?limit=0", "", "", "VALIDATION_FAILED", 422},
		{"excessive limit", "GET", myRidesPath + "?limit=101", "", "", "VALIDATION_FAILED", 422},
		{"bad cursor", "GET", myRidesPath + "?cursor=a%3Db", "", "", "INVALID_CURSOR", 400},
		{"unicode password", "POST", registerPath, registerCyrillicBody, "", "", 204},
		{"unicode whitespace", "POST", registerPath, registerNbspBody, "", "VALIDATION_FAILED", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", jsonMediaType)
			r.Header.Set("Origin", "http://127.0.0.1:8080")
			r.Header.Set("X-CSRF-Token", "example-csrf-value")
			if tc.key != "" {
				r.Header.Set("Idempotency-Key", tc.key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || tc.code != "" && !hasErrorCode(w, tc.code) {
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
	// Coordinates are longitude then latitude, so 91 is the out-of-range latitude here.
	body := `{
		"action_id": "` + commandID + `",
		"action": "set_position",
		"vehicle_id": "` + resourceID + `",
		"position": {"type": "Point", "coordinates": [74.6, 91]}
	}`
	r := httptest.NewRequest("POST", "/internal/v1/demo/actions", strings.NewReader(body))
	r.Header.Set("Content-Type", jsonMediaType)
	w := httptest.NewRecorder()
	contractRouter(t, spec).ServeHTTP(w, r)
	if w.Code != 422 || !strings.Contains(w.Body.String(), `/position/coordinates/1`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestInternalContractsAuthenticateBeforePayloadAndUseLargerBodyLimit(t *testing.T) {
	internalContracts := map[string]struct {
		load func() (*openapi3.T, error)
		path string
	}{
		"simulation": {internalapi.GetSwagger, "/internal/v1/simulation/tick"},
		"mailstub":   {mailstubapi.GetSwagger, "/internal/v1/messages"},
	}
	for name, contract := range internalContracts {
		t.Run(name, func(t *testing.T) {
			spec, err := contract.load()
			if err != nil {
				t.Fatal(err)
			}
			authenticate := func(_ context.Context, input *openapi3filter.AuthenticationInput) error {
				if input.RequestValidationInput.Request.Header.Get("Authorization") != testCredential {
					return fmt.Errorf("invalid test credential")
				}
				return nil
			}
			accepted := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			handler := boundary(spec, accepted, authenticate)
			overPublicLimit := `{"unexpected":"` + strings.Repeat("x", 64<<10) + `"}`
			overInternalLimit := strings.Repeat("x", 256<<10+1)
			for _, tc := range []struct {
				name, body, token string
				status            int
				code              string
			}{
				{"missing credentials before malformed JSON", "{", "", 401, "INTERNAL_AUTHENTICATION_REQUIRED"},
				{"invalid credentials before malformed JSON", "{", "Bearer wrong", 401, "INTERNAL_AUTHENTICATION_REQUIRED"},
				{"malformed authenticated JSON", "{", testCredential, 400, "MALFORMED_JSON"},
				{"more than public body limit", overPublicLimit, testCredential, 422, "VALIDATION_FAILED"},
				{"internal limit", overInternalLimit, testCredential, 413, "BODY_TOO_LARGE"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := httptest.NewRequest("POST", contract.path, strings.NewReader(tc.body))
					r.Header.Set("Content-Type", jsonMediaType)
					r.Header.Set("Authorization", tc.token)
					r.Header.Set("Delivery-Key", "invoice:"+resourceID+":issued")
					w := httptest.NewRecorder()
					handler.ServeHTTP(w, r)
					if w.Code != tc.status || !hasErrorCode(w, tc.code) {
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
		{"malformed JSON", `{"email":`, jsonMediaType, "", "MALFORMED_JSON", 400},
		{"unknown field", registerUnknownFieldBody, jsonMediaType, "", "VALIDATION_FAILED", 422},
		{"missing fields", `{}`, jsonMediaType, "", "VALIDATION_FAILED", 422},
		{"absent body", ``, jsonMediaType, "", "VALIDATION_FAILED", 422},
		{"invalid header", registerBody, jsonMediaType, "has a space", "INVALID_HEADER", 400},
		{"media type", `{}`, "text/plain", "", "UNSUPPORTED_MEDIA_TYPE", 415},
		{"body limit", strings.Repeat("x", 64<<10+1), jsonMediaType, "", "BODY_TOO_LARGE", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", registerPath, strings.NewReader(tc.body))
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
	handler := Router(Application{Probe: func(context.Context) (Status, error) { return Status{}, nil }})
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Extensions["x-implementation-status"] != "planned" {
				continue
			}
			w := httptest.NewRecorder()
			concrete := strings.ReplaceAll(path, "{id}", resourceID)
			handler.ServeHTTP(w, httptest.NewRequest(method, concrete, nil))
			if w.Code != 404 || !hasErrorCode(w, "RESOURCE_NOT_FOUND") {
				t.Errorf("%s %s: %d %s", method, path, w.Code, w.Body.String())
			}
		}
	}
}

// TestImplementedRoutesAreServed is the other half of TestPlannedRoutesRemainAbsentFromProduction:
// an operation the contract marks implemented must be reachable. Together the two tests keep the
// declared status and the router from drifting apart, in either direction.
func TestImplementedRoutesAreServed(t *testing.T) {
	spec, err := publicapi.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	handler := Router(Application{Probe: func(context.Context) (Status, error) { return Status{}, nil }})
	served := 0
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Extensions["x-implementation-status"] != "implemented" {
				continue
			}
			served++
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code == 404 && hasErrorCode(w, "RESOURCE_NOT_FOUND") {
				t.Errorf("%s %s is marked implemented but is not routed", method, path)
			}
		}
	}
	if served == 0 {
		t.Fatal("no operation is marked implemented, so this test proved nothing")
	}
}

func hasErrorCode(w *httptest.ResponseRecorder, code string) bool {
	return strings.Contains(w.Body.String(), `"code":"`+code+`"`)
}
