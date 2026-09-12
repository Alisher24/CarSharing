package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
)

func TestHealthSeparatesProcessFromDependencies(t *testing.T) {
	for _, path := range []string{"/api/v1/health/live", "/api/v1/health/ready"} {
		t.Run(path, func(t *testing.T) {
			probed := false
			r := Router(Application{Probe: func(ctx context.Context) (servedapi.ReadyStatus, error) {
				probed = true
				if _, ok := ctx.Deadline(); !ok {
					t.Error("readiness check has no deadline")
				}
				return servedapi.ReadyStatus{}, errors.New("private database error")
			}})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			want := 503
			if path == "/api/v1/health/live" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("status = %d, want %d", w.Code, want)
			}
			if probed != (path != "/api/v1/health/live") {
				t.Error("liveness queried the database")
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Error("dependency error leaked")
			}
		})
	}
}

func TestReadyReturnsServerMetadataWithoutCaching(t *testing.T) {
	r := Router(Application{Probe: func(context.Context) (servedapi.ReadyStatus, error) {
		return servedapi.ReadyStatus{City: "Бишкек", Currency: "KGS", Timezone: "Asia/Bishkek"}, nil
	}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/health/ready", nil))
	var body servedapi.ReadyStatus
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || body.Status != "ok" || body.Currency != "KGS" || body.City != "Бишкек" {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
	if _, err := time.Parse(time.RFC3339Nano, body.ServerTime); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get(cacheControlHeader) != noStoreCacheControl {
		t.Error("health must not be cached")
	}
}
