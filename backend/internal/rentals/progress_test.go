package rentals

import (
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
)

// The estimate prices the minutes begun in each mode at the rates stored with the rental, so a
// catalog that moved after the reservation changes nothing about what the ride has cost so far.
func TestEstimateUsesTheStoredRates(t *testing.T) {
	frozen := tariffs.Tariff{
		DrivingRateTyiynPerStartedMinute: 1234,
		PausedRateTyiynPerStartedMinute:  321,
	}
	for _, ride := range []struct {
		name            string
		driving, paused int64
		amount          int64
	}{
		{"nothing begun", 0, 0, 0},
		{"one driving minute", 1, 0, 1234},
		{"one paused minute", 0, 1, 321},
		{"the example of the specification", 2, 1, 2789},
		{"two paused minutes", 0, 2, 642},
	} {
		if got := estimatedAmount(frozen, ride.driving, ride.paused); got != ride.amount {
			t.Errorf("%s costs %d tyiyn, want %d", ride.name, got, ride.amount)
		}
	}
}

// Two rates read by the same sum: an amount larger than the exact range of a double is still exact in
// tiyin, because the whole computation is integer arithmetic over the stored rates.
func TestEstimateKeepsAmountsBeyondTheExactDoubleRange(t *testing.T) {
	price := tariffs.Tariff{DrivingRateTyiynPerStartedMinute: 9_007_199_254_740_993}
	got := estimatedAmount(price, 1, 0)
	if got != 9_007_199_254_740_993 {
		t.Fatalf("the estimate is %d, want the exact stored product", got)
	}
}
