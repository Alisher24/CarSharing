package fleet

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type recordedVersionRow struct {
	version int64
}

func (r recordedVersionRow) Scan(destinations ...any) error {
	*destinations[0].(*int64) = r.version
	return nil
}

type recordingVehicleTransaction struct {
	pgx.Tx
	statement string
	arguments []any
	version   int64
}

func (t *recordingVehicleTransaction) QueryRow(
	_ context.Context,
	statement string,
	arguments ...any,
) pgx.Row {
	t.statement = statement
	t.arguments = arguments
	return recordedVersionRow{version: t.version}
}

func TestPublishingAVehicleChangeRaisesItsStoredVersion(t *testing.T) {
	transaction := &recordingVehicleTransaction{version: 8}
	store := &Store{}

	version, err := store.PublishChange(
		context.Background(),
		transaction,
		"01994342-6ba7-7000-8000-000000000001",
		VehicleChange{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if version != transaction.version {
		t.Fatalf("version = %d, want %d", version, transaction.version)
	}
	if count := strings.Count(transaction.statement, "version = version + 1"); count != 1 {
		t.Fatalf("version rule occurs %d times in the owner statement", count)
	}
	if got := transaction.arguments[0]; got != "01994342-6ba7-7000-8000-000000000001" {
		t.Fatalf("vehicle identifier = %v", got)
	}
}
