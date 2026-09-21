package rentals

import (
	"strings"
	"testing"
)

func TestInstallationMetadataHasOneStorageRead(t *testing.T) {
	if count := strings.Count(installationMetadataSelection, "bootstrap_metadata"); count != 1 {
		t.Fatalf("installation metadata storage reads = %d, want 1", count)
	}
	if strings.Contains(dailyLimitStatement, "bootstrap_metadata") {
		t.Fatal("daily-limit query reads installation metadata outside the owning operation")
	}
}

func TestRentalWritesShareTheStoredFieldDeclaration(t *testing.T) {
	statements := map[string]string{
		"reservation insert": insertReservationStatement,
		"prepared insert":    insertPreparedRentalStatement,
		"restoration insert": restorePreparedRentalStatement,
		"ride start":         startRideStatement,
		"mode change":        changeModeStatement,
		"reservation end":    releaseRentalStatement,
		"ride completion":    completeRentalStatement,
	}
	for name, statement := range statements {
		if count := strings.Count(statement, rentalFields); count != 1 {
			t.Errorf("%s contains the shared rental fields %d times, want 1", name, count)
		}
	}
}
