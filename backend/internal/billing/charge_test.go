package billing

import (
	"testing"
	"time"
)

// The demo rates of the specification, in whole tyiyn per begun minute of each mode. The expected
// amounts below are the ones the table states, not products of these two numbers computed here.
var demoRates = Rates{Driving: 1234, Paused: 321}

// oneSecond and oneMicrosecond name the units the table is written in, so a row states the time it
// means rather than a count of microseconds a reader has to convert.
const (
	oneSecond      = time.Second
	oneMicrosecond = time.Microsecond
)

// mode is the time one ride spent in one mode: the intervals it was made of, in the order they were
// driven or stood still. Grouping matters: three intervals of twenty seconds are summed before the
// rounding, which is what makes them one begun minute rather than three.
type mode []time.Duration

func (m mode) total() time.Duration {
	var total time.Duration
	for _, interval := range m {
		total += interval
	}
	return total
}

// The M01–M11 examples of the money section of the product design, each stated as the intervals of
// the ride it describes. Every row names what the ride did and asserts the minutes begun in each mode
// and the amount those minutes cost, so the policy is checked against the table rather than against
// itself.
func TestTheExamplesOfThePolicy(t *testing.T) {
	for _, example := range []struct {
		id              string
		driving, paused mode
		drivingMinutes  int64
		pausedMinutes   int64
		amountTyiyn     int64
	}{
		{id: "M01", driving: nil, paused: nil, drivingMinutes: 0, pausedMinutes: 0, amountTyiyn: 0},
		{
			id: "M02", driving: mode{oneMicrosecond}, paused: nil,
			drivingMinutes: 1, pausedMinutes: 0, amountTyiyn: 1234,
		},
		{
			id: "M03", driving: mode{oneSecond}, paused: nil,
			drivingMinutes: 1, pausedMinutes: 0, amountTyiyn: 1234,
		},
		{
			id: "M04", driving: mode{60 * oneSecond}, paused: nil,
			drivingMinutes: 1, pausedMinutes: 0, amountTyiyn: 1234,
		},
		{
			id: "M05", driving: mode{60*oneSecond + oneMicrosecond}, paused: nil,
			drivingMinutes: 2, pausedMinutes: 0, amountTyiyn: 2468,
		},
		{
			id: "M06", driving: mode{61 * oneSecond}, paused: nil,
			drivingMinutes: 2, pausedMinutes: 0, amountTyiyn: 2468,
		},
		{
			id: "M07", driving: mode{90 * oneSecond}, paused: mode{45 * oneSecond},
			drivingMinutes: 2, pausedMinutes: 1, amountTyiyn: 2789,
		},
		{
			id: "M08", driving: mode{20 * oneSecond, 20 * oneSecond}, paused: mode{10 * oneSecond},
			drivingMinutes: 1, pausedMinutes: 1, amountTyiyn: 1555,
		},
		{
			id:      "M09",
			driving: mode{20 * oneSecond, 20 * oneSecond, 20 * oneSecond},
			paused:  mode{10 * oneSecond, 10 * oneSecond},
			// Three driving intervals and two paused ones, each below a minute: rounding every
			// interval separately would answer three begun minutes of driving instead of one.
			drivingMinutes: 1, pausedMinutes: 1, amountTyiyn: 1555,
		},
		{
			id: "M10",
			driving: mode{
				20 * oneSecond, 20 * oneSecond, 20*oneSecond + oneMicrosecond,
			},
			paused:         mode{30 * oneSecond, 30 * oneSecond},
			drivingMinutes: 2, pausedMinutes: 1, amountTyiyn: 2789,
		},
		{
			id: "M11", driving: nil, paused: mode{60*oneSecond + oneMicrosecond},
			drivingMinutes: 0, pausedMinutes: 2, amountTyiyn: 642,
		},
	} {
		t.Run(example.id, func(t *testing.T) {
			charge, err := Compute(demoRates, example.driving.total(), example.paused.total())
			if err != nil {
				t.Fatalf("the example was refused: %v", err)
			}
			if charge.DrivingMinutes != example.drivingMinutes {
				t.Errorf("the driving mode has begun %d minutes, want %d", charge.DrivingMinutes, example.drivingMinutes)
			}
			if charge.PausedMinutes != example.pausedMinutes {
				t.Errorf("the paused mode has begun %d minutes, want %d", charge.PausedMinutes, example.pausedMinutes)
			}
			if charge.TotalTyiyn != example.amountTyiyn {
				t.Errorf("the ride costs %d tyiyn, want %d", charge.TotalTyiyn, example.amountTyiyn)
			}
		})
	}
}

// The rounding rule at the boundary the specification states it for: a zero duration has begun no
// minute, sixty seconds have begun exactly one, and one microsecond past a minute has begun the next.
func TestStartedMinutesRoundsEachSumUpOnce(t *testing.T) {
	for _, began := range []struct {
		duration time.Duration
		minutes  int64
	}{
		{duration: 0, minutes: 0},
		{duration: oneMicrosecond, minutes: 1},
		{duration: oneSecond, minutes: 1},
		{duration: 60 * oneSecond, minutes: 1},
		{duration: 60*oneSecond + oneMicrosecond, minutes: 2},
		{duration: 61 * oneSecond, minutes: 2},
		{duration: 119*oneSecond + 999999*oneMicrosecond, minutes: 2},
		{duration: 120 * oneSecond, minutes: 2},
	} {
		minutes, err := StartedMinutes(began.duration)
		if err != nil {
			t.Fatalf("%s was refused: %v", began.duration, err)
		}
		if minutes != began.minutes {
			t.Errorf("%s has begun %d minutes, want %d", began.duration, minutes, began.minutes)
		}
	}
}

// The charge of a mode is the rate of that mode times the minutes begun in it, and the total is the
// two charges added. One charge alone is stated here, because Compute is checked against the table.
func TestChargePricesTheMinutesOfItsOwnMode(t *testing.T) {
	for _, priced := range []struct {
		name    string
		rate    int64
		minutes int64
		amount  int64
	}{
		{name: "nothing begun", rate: 1234, minutes: 0, amount: 0},
		{name: "one driving minute", rate: 1234, minutes: 1, amount: 1234},
		{name: "one paused minute", rate: 321, minutes: 1, amount: 321},
		{name: "two paused minutes", rate: 321, minutes: 2, amount: 642},
	} {
		amount, err := demoRates.Charge(priced.rate, priced.minutes)
		if err != nil {
			t.Fatalf("%s was refused: %v", priced.name, err)
		}
		if amount != priced.amount {
			t.Errorf("%s costs %d tyiyn, want %d", priced.name, amount, priced.amount)
		}
	}
}
