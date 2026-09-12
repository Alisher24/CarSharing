package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

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
