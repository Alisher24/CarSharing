package rentals

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestRentalTransactionsDeclareOneLockOrder(t *testing.T) {
	planned := participants{
		users:    []uuid.UUID{uuid.MustParse("01994342-6ba7-7000-8000-000000000001")},
		vehicles: []string{"vehicle"},
		rentals:  []string{"rental"},
	}
	locks := planned.locks()
	names := make([]string, 0, len(locks))
	for _, target := range locks {
		names = append(names, target.name)
	}

	want := []string{"users", "vehicles", "rentals"}
	if !slices.Equal(names, want) {
		t.Fatalf("lock order = %v, want %v", names, want)
	}
}
