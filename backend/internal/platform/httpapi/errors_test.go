package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	healthapi "github.com/Alisher24/CarSharing/backend/internal/contracts/healthapi"
)

func TestEveryTransportCodeDeclaresAContractStatus(t *testing.T) {
	for code, status := range transportErrorStatus {
		if !code.Valid() {
			t.Errorf("%s is not declared by the contract", code)
		}
		if status < http.StatusBadRequest || status > 599 {
			t.Errorf("%s maps to %d, which is not a failure status", code, status)
		}
	}
}

// An unmapped code would otherwise write status 0, which the standard library rejects at runtime.
func TestUnmappedCodeFallsBackToInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, httptest.NewRequest("GET", "/", nil), healthapi.VEHICLEUNAVAILABLE, "Vehicle unavailable")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	var body healthapi.ApiError
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != healthapi.VEHICLEUNAVAILABLE {
		t.Fatalf("code = %s, want %s", body.Code, healthapi.VEHICLEUNAVAILABLE)
	}
}
