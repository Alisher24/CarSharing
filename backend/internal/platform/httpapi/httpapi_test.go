package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthSeparatesProcessFromDependencies(t *testing.T) {
	for _, path := range []string{"/api/v1/health/live", "/api/v1/health/ready"} {
		t.Run(path, func(t *testing.T) {
			called := false
			r := Router(func(ctx context.Context) (Status, error) {
				called = true
				if _, ok := ctx.Deadline(); !ok {
					t.Error("readiness check has no deadline")
				}
				return Status{}, errors.New("private database error")
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			want := 503
			if path == "/api/v1/health/live" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("status = %d, want %d", w.Code, want)
			}
			if called != (path != "/api/v1/health/live") {
				t.Error("liveness queried the database")
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Error("dependency error leaked")
			}
		})
	}
}

func TestReadyReturnsServerMetadataWithoutCaching(t *testing.T) {
	r := Router(func(context.Context) (Status, error) {
		return Status{City: "Бишкек", Currency: "KGS", Timezone: "Asia/Bishkek"}, nil
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/health/ready", nil))
	var body Status
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || body.Status != "ok" || body.Currency != "KGS" || body.City != "Бишкек" {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
	if _, err := time.Parse(time.RFC3339Nano, body.ServerTime); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("health must not be cached")
	}
}

func TestRoutingErrorsUseTheAPIErrorContract(t *testing.T) {
	for _, tc := range []struct {
		method, path, code string
		status             int
	}{
		{"GET", "/api/health", "RESOURCE_NOT_FOUND", 404},
		{"GET", "/health/live", "RESOURCE_NOT_FOUND", 404},
		{"GET", "/health/ready", "RESOURCE_NOT_FOUND", 404},
		{"GET", "/api/v1/vehicles", "RESOURCE_NOT_FOUND", 404},
		{"POST", "/api/v1/health/live", "METHOD_NOT_ALLOWED", 405},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := Router(func(context.Context) (Status, error) { return Status{}, nil })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			var body struct {
				Code      string `json:"code"`
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body.Code != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if body.RequestID == "" || body.RequestID != w.Header().Get("X-Request-ID") {
				t.Fatal("request IDs disagree")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("error can be cached")
			}
			if tc.status == 405 && w.Header().Get("Allow") != "GET" {
				t.Fatal("missing allowed method")
			}
		})
	}
}
