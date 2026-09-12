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
	for _, path := range []string{"/health/live", "/health/ready", "/api/health"} {
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
			if path == "/health/live" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("status = %d, want %d", w.Code, want)
			}
			if called != (path != "/health/live") {
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
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/health", nil))
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
