package billing

import (
	"testing"
	"time"
)

// halfTheRange is the largest value whose double fits twice in the signed 64-bit range. Two of them
// are the pair the design states as overflowing the total while each product fits on its own.
const halfTheRange int64 = 1 << 62

// A duration cannot be negative: a ride that took less than no time is not one the arithmetic can
// price, and answering it with zero minutes would publish a ride that cost nothing.
func TestANegativeDurationIsRefused(t *testing.T) {
	for _, refused := range []struct {
		name            string
		driving, paused time.Duration
	}{
		{name: "driving before the ride began", driving: -1, paused: 0},
		{name: "paused before the ride began", driving: 0, paused: -1 * time.Microsecond},
	} {
		minutes, err := StartedMinutes(-1)
		if err == nil {
			t.Errorf("%s: the negative duration has begun %d minutes", refused.name, minutes)
		}
		charge, err := Compute(demoRates, refused.driving, refused.paused)
		if err == nil {
			t.Errorf("%s: the ride was priced at %d tyiyn", refused.name, charge.TotalTyiyn)
		}
		if charge != (Charge{}) {
			t.Errorf("%s: a refusal carried the partial charge %+v", refused.name, charge)
		}
	}
}

// A rate nobody could set is reported rather than subtracted from the bill, and the refusal carries no
// amount a caller could publish.
func TestANegativeRateIsRefused(t *testing.T) {
	for _, refused := range []struct {
		name  string
		rates Rates
	}{
		{name: "driving", rates: Rates{Driving: -1}},
		{name: "paused", rates: Rates{Paused: -1}},
	} {
		charge, err := Compute(refused.rates, time.Minute, time.Minute)
		if err == nil {
			t.Errorf("the negative %s rate priced the ride at %d tyiyn", refused.name, charge.TotalTyiyn)
		}
		if charge != (Charge{}) {
			t.Errorf("the negative %s rate carried the partial charge %+v", refused.name, charge)
		}
	}
}

// One minute at a rate equal to the largest representable value is exactly that value; a second begun
// minute is not representable, so it is refused rather than answered with the digits a wrap leaves.
func TestTheLargestRatePaysForOneMinuteAndNoMore(t *testing.T) {
	price := Rates{Driving: maxRepresentable}

	charge, err := Compute(price, time.Minute, 0)
	if err != nil {
		t.Fatalf("one minute at the largest rate was refused: %v", err)
	}
	if charge.TotalTyiyn != maxRepresentable {
		t.Errorf("one minute costs %d tyiyn, want %d", charge.TotalTyiyn, maxRepresentable)
	}

	refused, err := Compute(price, 2*time.Minute, 0)
	if err == nil {
		t.Fatalf("two minutes at the largest rate were priced at %d tyiyn", refused.TotalTyiyn)
	}
	if refused != (Charge{}) {
		t.Errorf("the refusal carried the partial charge %+v", refused)
	}
}

// Two amounts that each fit can still add up to one that does not, so the total is checked as well as
// each product.
func TestATotalBeyondTheRangeIsRefused(t *testing.T) {
	price := Rates{Driving: halfTheRange, Paused: halfTheRange}

	refused, err := Compute(price, time.Minute, time.Minute)
	if err == nil {
		t.Fatalf("the total was priced at %d tyiyn", refused.TotalTyiyn)
	}
	if refused != (Charge{}) {
		t.Errorf("the refusal carried the partial charge %+v", refused)
	}

	// One of the two products alone is representable, so the refusal above is the sum and not a
	// product: the total is what leaves the range.
	if _, err = Compute(price, time.Minute, 0); err != nil {
		t.Errorf("one amount of half the range was refused: %v", err)
	}
}

// The microseconds of one mode are added before they are rounded, and a total beyond the signed
// 64-bit range is refused before it is used.
func TestADurationSumBeyondTheRangeIsRefused(t *testing.T) {
	largest := int64(maxRepresentable)

	total, err := SumMicroseconds(largest, 0)
	if err != nil {
		t.Fatalf("the largest representable duration was refused: %v", err)
	}
	if total != largest {
		t.Errorf("the sum is %d microseconds, want %d", total, largest)
	}

	total, err = SumMicroseconds(largest, 1)
	if err == nil {
		t.Fatalf("a duration past the range was answered as %d microseconds", total)
	}
	if total != 0 {
		t.Errorf("the refusal answered %d microseconds", total)
	}

	for _, negative := range []int64{-1, -int64(time.Microsecond)} {
		if _, err = SumMicroseconds(negative, 0); err == nil {
			t.Errorf("the negative duration %d microseconds was summed", negative)
		}
	}
}
