package rentals

import (
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
)

// A ride of several intervals is summed per mode and rounded once, so the begun minutes and the
// amount are the ones the billing module computes for the same durations and the same stored rates.
// The intervals of M09 are stated here as the rental module holds them: three driving intervals below
// a minute with two ten-second pauses, which are one begun minute of each mode.
func TestProgressIsBuiltFromTheIntervalsOfTheRide(t *testing.T) {
	intervals := modeDurations{
		driving: 20*time.Second + 20*time.Second + 20*time.Second,
		paused:  10*time.Second + 10*time.Second,
	}
	ride := Rental{Tariff: ratesSnapshot(1234, 321)}

	want, err := billing.Compute(billing.Rates{Driving: 1234, Paused: 321},
		intervals.driving, intervals.paused)
	if err != nil {
		t.Fatalf("the intervals were refused: %v", err)
	}
	got := progressOf(t, ride, intervals)
	if got.DrivingMinutes != want.DrivingMinutes || got.PausedMinutes != want.PausedMinutes {
		t.Errorf("the ride has begun %d driving and %d paused minutes, want %d and %d",
			got.DrivingMinutes, got.PausedMinutes, want.DrivingMinutes, want.PausedMinutes)
	}
	if got.TotalTyiyn != want.TotalTyiyn {
		t.Errorf("the ride costs %d tyiyn, want %d", got.TotalTyiyn, want.TotalTyiyn)
	}
	if got.DrivingDuration != intervals.driving || got.PausedDuration != intervals.paused {
		t.Errorf("the published durations are %s and %s, want %s and %s",
			got.DrivingDuration, got.PausedDuration, intervals.driving, intervals.paused)
	}
}

// The example of the specification costs 27,89 som: ninety seconds of driving at 12,34 som a minute
// and forty-five seconds of standing still at 3,21, which are two begun minutes and one.
func TestProgressCostsTheExampleOfTheSpecification(t *testing.T) {
	ride := Rental{Tariff: ratesSnapshot(1234, 321)}
	got := progressOf(t, ride, modeDurations{driving: 90 * time.Second, paused: 45 * time.Second})
	if got.TotalTyiyn != 2789 {
		t.Errorf("the ride costs %d tyiyn, want 2789", got.TotalTyiyn)
	}
}

// The amount is priced at the rates stored with the rental, so a catalog that moved after the
// reservation changes nothing about what the ride costs.
func TestProgressUsesTheSnapshotRatherThanTheCatalog(t *testing.T) {
	frozen := Rental{Tariff: ratesSnapshot(1234, 321)}
	moved := Rental{Tariff: ratesSnapshot(2000, 500)}
	intervals := modeDurations{driving: 90 * time.Second, paused: 45 * time.Second}

	if got := progressOf(t, frozen, intervals); got.TotalTyiyn != 2789 {
		t.Errorf("the ride under the stored rates costs %d tyiyn, want 2789", got.TotalTyiyn)
	}
	if got := progressOf(t, moved, intervals); got.TotalTyiyn != 4500 {
		t.Errorf("the ride under the moved rates costs %d tyiyn, want 4500", got.TotalTyiyn)
	}
}

// An amount larger than the exact range of a double is still exact in tiyin, because the whole
// computation is integer arithmetic over the stored rates.
func TestProgressKeepsAmountsBeyondTheExactDoubleRange(t *testing.T) {
	ride := Rental{Tariff: ratesSnapshot(9_007_199_254_740_993, 0)}
	got := progressOf(t, ride, modeDurations{driving: time.Minute})
	if got.TotalTyiyn != 9_007_199_254_740_993 {
		t.Errorf("the estimate is %d, want the exact stored product", got.TotalTyiyn)
	}
}

// A charge the billing module refuses is reported rather than published as an amount of its own, and
// the same answer is what an invoice of that ride would be written from: a ride nobody can price is a
// ride nobody can end.
func TestProgressReportsAnAmountItCannotRepresent(t *testing.T) {
	ride := Rental{Tariff: ratesSnapshot(9_223_372_036_854_775_807, 0)}
	if _, err := ride.priceOf(modeDurations{driving: 2 * time.Minute}); err == nil {
		t.Error("two minutes at the largest rate were priced rather than refused")
	}
}

// progressOf is what one rental publishes for one set of durations, which every check above asserts
// against: the durations the intervals summed to, priced at the rates the rental stored.
func progressOf(t *testing.T, ride Rental, intervals modeDurations) billing.Charge {
	t.Helper()
	progress, err := ride.priceOf(intervals)
	if err != nil {
		t.Fatalf("the ride was refused a progress: %v", err)
	}
	return progress
}

// ratesSnapshot is the price list a rental stored when it was reserved, of which only the two rates
// matter to a charge.
func ratesSnapshot(driving, paused int64) tariffs.Tariff {
	return tariffs.Tariff{
		BillingPolicy:                    billing.PolicyPerModeStartedMinuteV1,
		DrivingRateTyiynPerStartedMinute: driving,
		PausedRateTyiynPerStartedMinute:  paused,
	}
}
