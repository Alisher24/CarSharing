package billing

import (
	"testing"
	"time"
)

// One ride is written down as two lines, driving first and paused second, and the amount of each is
// the share of the total that mode contributed. The lines of the M07 example are stated here as the
// invoice of that ride states them.
func TestTheLinesOfAChargeAreTheTwoModesItWasComputedFrom(t *testing.T) {
	charge, err := Compute(demoRates, 90*oneSecond, 45*oneSecond)
	if err != nil {
		t.Fatalf("the example was refused: %v", err)
	}

	driving, paused := charge.Lines()
	for _, line := range []struct {
		name     string
		line     Line
		mode     Mode
		duration time.Duration
		minutes  int64
		rate     RateTyiynPerStartedMinute
		amount   AmountTyiyn
	}{
		{
			name: "driving", line: driving, mode: Driving, duration: 90 * oneSecond,
			minutes: 2, rate: 1234, amount: 2468,
		},
		{
			name: "paused", line: paused, mode: Paused, duration: 45 * oneSecond,
			minutes: 1, rate: 321, amount: 321,
		},
	} {
		if line.line.Mode != line.mode {
			t.Errorf("the %s line describes %q", line.name, line.line.Mode)
		}
		if line.line.Duration != line.duration {
			t.Errorf("the %s line lasts %s, want %s", line.name, line.line.Duration, line.duration)
		}
		if line.line.Minutes != line.minutes {
			t.Errorf("the %s line bills %d minutes, want %d", line.name, line.line.Minutes, line.minutes)
		}
		if line.line.Rate != line.rate {
			t.Errorf("the %s line is priced at %d, want %d", line.name, line.line.Rate, line.rate)
		}
		if line.line.AmountTyiyn != line.amount {
			t.Errorf("the %s line costs %d tyiyn, want %d", line.name, line.line.AmountTyiyn, line.amount)
		}
	}

	// The total of the charge is the sum of the two lines it publishes, which is what makes an
	// invoice add up for a reader rather than only for the service that wrote it.
	if driving.AmountTyiyn+paused.AmountTyiyn != charge.TotalTyiyn {
		t.Errorf("the lines add up to %d tyiyn, want the total %d",
			driving.AmountTyiyn+paused.AmountTyiyn, charge.TotalTyiyn)
	}
	if charge.TotalTyiyn != 2789 {
		t.Errorf("the example costs %d tyiyn, want 2789", charge.TotalTyiyn)
	}
}

// A mode a ride never entered still has a line, with a zero duration and a zero amount: the contract
// states exactly two lines, so an invoice of a ride that only drove states a pause of nothing rather
// than leaving it out.
func TestAModeTheRideNeverEnteredIsStillALine(t *testing.T) {
	charge, err := Compute(demoRates, oneSecond, 0)
	if err != nil {
		t.Fatalf("the ride was refused: %v", err)
	}

	_, paused := charge.Lines()
	if paused.Mode != Paused {
		t.Errorf("the second line describes %q", paused.Mode)
	}
	if paused.Duration != 0 || paused.Minutes != 0 || paused.AmountTyiyn != 0 {
		t.Errorf("the pause of a ride that never paused is %+v", paused)
	}
}

// A duration is published as whole microseconds, which is the unit the contract states and the unit a
// line stores: a duration measured in nanoseconds cannot hold every count the contract admits.
func TestALineStatesItsDurationInWholeMicroseconds(t *testing.T) {
	line := Line{Mode: Driving, Duration: 60*oneSecond + oneMicrosecond}
	if got := line.Microseconds(); got != 60_000_001 {
		t.Errorf("the line states %d microseconds, want 60000001", got)
	}
}

// The two lines of a charge that each fit can still add up to a total that does not, so the total is
// checked rather than added blindly. The refusal is the one Compute answers, because a charge is only
// ever totalled where it is computed.
func TestATotalOfTwoAmountsBeyondTheRangeIsRefused(t *testing.T) {
	half := AmountTyiyn(maxRepresentable/2 + 1)
	if _, err := Add(half, half); err == nil {
		t.Error("two amounts of more than half the range were totalled rather than refused")
	}

	// The largest representable amount added to nothing is still representable, so the refusal above is
	// the sum leaving the range rather than either amount.
	if total, err := Add(AmountTyiyn(maxRepresentable), 0); err != nil || total != maxRepresentable {
		t.Errorf("the largest amount was answered %d with %v", total, err)
	}
}
