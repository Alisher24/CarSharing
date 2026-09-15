package simulation_test

import (
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
)

// epoch is the moment every journey in these checks begins. Nothing here reads a clock: the model is
// moved to the moment it is given, which is what makes the same arguments produce the same answer.
var epoch = time.Date(2026, time.September, 14, 9, 0, 0, 0, time.UTC)

// whole is one full source, in millionths of its unit.
const whole = fleet.Amount(fleet.AmountScale)

// capacity is the demonstration capacity of one source kind, in millionths of its unit.
var capacity = map[fleet.SourceKind]fleet.Amount{
	fleet.SourceBattery:  60_000 * whole,
	fleet.SourceGasoline: 50_000 * whole,
	fleet.SourceLPG:      60_000 * whole,
}

// lasting is what a source of this capacity holds to last this long of driving. A check states how long
// a reserve lasts rather than how many millionths it is, because the rules being checked are about
// time: a sixtieth of a source is a minute, and a tenth of one is six minutes. The part of a millionth
// a division leaves is rounded up rather than away, so a reserve stated for a duration covers the whole
// of it instead of ending a fraction short of it.
func lasting(kind fleet.SourceKind, lasts time.Duration) fleet.Amount {
	held := new(big.Int).Mul(big.NewInt(int64(capacity[kind])), big.NewInt(lasts.Nanoseconds()))
	window := big.NewInt(periodOfDriving)
	held.Add(held, new(big.Int).Sub(window, big.NewInt(1)))
	held.Quo(held, window)
	return fleet.Amount(held.Int64())
}

// periodOfDriving is the window a whole source lasts, which is the hour a capacity is stated over. It is
// the model's own unit, written here because a check that states a reserve as a duration needs it, so
// the two must agree on it.
const periodOfDriving = 3_600_000_000_000

// vehicle is a state the checks start from: the reserves they name, standing at the beginning of its
// route at epoch.
func vehicle(powertrain fleet.PowertrainType, reserves map[fleet.SourceKind]fleet.Amount) simulation.State {
	profile, known := fleet.ProfileOf(powertrain)
	if !known {
		panic("the check names a powertrain the fleet does not carry")
	}
	state := simulation.State{
		RouteID:     "gas-1",
		Parameters:  simulation.ParametersOf(capacity),
		ProcessedAt: epoch,
	}
	route, known := simulation.RouteOf(state.RouteID)
	if !known {
		panic("the check names a route this build does not declare")
	}
	state.Position = route.PositionAt(0)
	for _, kind := range profile.Sources {
		state.Sources = append(state.Sources, simulation.SourceWith(kind, capacity[kind], reserves[kind]))
	}
	return state
}

// spent is what one source of a state lost since the state it started from.

// driving is one unbroken window of movement.
func driving(duration time.Duration) []simulation.Span {
	return []simulation.Span{{Mode: simulation.Driving, From: epoch, To: epoch.Add(duration)}}
}

// remaining is what one source of a state holds, in millionths of its unit.
func remaining(state simulation.State, kind fleet.SourceKind) fleet.Amount {
	for index, source := range state.Sources {
		if source.Kind == kind {
			return state.Holding()[index]
		}
	}
	return -1
}

// spent is what one source of a state lost since the state it started from.
func spent(moved, before simulation.State, kind fleet.SourceKind) fleet.Amount {
	return remaining(before, kind) - remaining(moved, kind)
}

func depletedAt(motion simulation.Motion) time.Time {
	at, _ := motion.DepletedBy()
	return at
}

func TestProfileOrderIsTheAgreedOne(t *testing.T) {
	agreed := map[fleet.PowertrainType][]fleet.SourceKind{
		fleet.PowertrainElectric: {fleet.SourceBattery},
		fleet.PowertrainGasoline: {fleet.SourceGasoline},
		fleet.PowertrainDiesel:   {fleet.SourceDiesel},
		fleet.PowertrainHybrid:   {fleet.SourceBattery, fleet.SourceGasoline},
		fleet.PowertrainGas:      {fleet.SourceLPG, fleet.SourceGasoline},
	}
	if len(fleet.PowertrainTypes()) != len(agreed) {
		t.Fatal("the fleet carries a powertrain this check states no order for")
	}
	for _, powertrain := range fleet.PowertrainTypes() {
		profile, known := fleet.ProfileOf(powertrain)
		if !known {
			t.Fatalf("%s has no profile", powertrain)
		}
		if !sameKinds(profile.Sources, agreed[powertrain]) {
			t.Errorf("%s uses %v, the agreed order is %v", powertrain, profile.Sources, agreed[powertrain])
		}
	}
}

func sameKinds(left, right []fleet.SourceKind) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestOneSourceLastsExactlySixtyMinutesOfDriving(t *testing.T) {
	start := vehicle(fleet.PowertrainElectric, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery: capacity[fleet.SourceBattery],
	})
	hour, spentHour, err := start.Journey(driving(60*time.Minute), epoch.Add(60*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if left := remaining(spentHour, fleet.SourceBattery); left != 0 {
		t.Errorf("a whole battery holds %d after sixty minutes of driving", left)
	}
	if got, want := depletedAt(hour), epoch.Add(60*time.Minute); !got.Equal(want) {
		t.Errorf("a battery spent to nothing ran out at %v, the arithmetic says %v", got, want)
	}

	short, spentShort, err := start.Journey(driving(59*time.Minute), epoch.Add(59*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if left := remaining(spentShort, fleet.SourceBattery); left <= 0 {
		t.Errorf("a battery holds %d after fifty-nine minutes of driving", left)
	}
	if _, depleted := short.DepletedBy(); depleted {
		t.Error("a battery with a minute of driving left is reported as spent")
	}
}

func TestPausedConsumptionIsATenthOfDriving(t *testing.T) {
	// A tenth of a battery lasts a tenth of an hour of driving, and ten times that standing still, so
	// the moment the vehicle runs out is the whole of the rule.
	start := vehicle(fleet.PowertrainElectric, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery: lasting(fleet.SourceBattery, time.Hour),
	})
	moved, _, err := start.Journey(driving(60*time.Minute), epoch.Add(60*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	stood, _, err := start.Journey([]simulation.Span{
		{Mode: simulation.Paused, From: epoch, To: epoch.Add(10 * time.Hour)},
	}, epoch.Add(10*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := depletedAt(moved), epoch.Add(60*time.Minute); !got.Equal(want) {
		t.Errorf("a tenth of a battery ran out at %v while driving, the arithmetic says %v", got, want)
	}
	if got, want := depletedAt(stood), epoch.Add(10*time.Hour); !got.Equal(want) {
		t.Errorf("a tenth of a battery ran out at %v while standing, the arithmetic says %v", got, want)
	}
}

func TestAFreeVehicleSpendsNothingAndStaysWhereItIs(t *testing.T) {
	start := vehicle(fleet.PowertrainElectric, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery: capacity[fleet.SourceBattery],
	})
	_, moved, err := start.Journey([]simulation.Span{
		{Mode: simulation.Free, From: epoch, To: epoch.Add(time.Hour)},
	}, epoch.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if lost := spent(moved, start, fleet.SourceBattery); lost != 0 {
		t.Errorf("a vehicle nobody is driving spent %d", lost)
	}
	if moved.Position != start.Position {
		t.Errorf("a free vehicle moved from %v to %v", start.Position, moved.Position)
	}
}

func TestRunningOutSwitchesToTheReserveInBothOrders(t *testing.T) {
	for _, check := range []struct {
		name       string
		powertrain fleet.PowertrainType
		first      fleet.SourceKind
		reserve    fleet.SourceKind
	}{
		{"hybrid", fleet.PowertrainHybrid, fleet.SourceBattery, fleet.SourceGasoline},
		{"gas", fleet.PowertrainGas, fleet.SourceLPG, fleet.SourceGasoline},
	} {
		t.Run(check.name, func(t *testing.T) {
			// A sixtieth of the priority source lasts one minute of driving, so the four minutes after
			// it are covered by the reserve.
			start := vehicle(check.powertrain, map[fleet.SourceKind]fleet.Amount{
				check.first:   lasting(check.first, time.Minute),
				check.reserve: capacity[check.reserve],
			})
			motion, moved, err := start.Journey(driving(5*time.Minute), epoch.Add(5*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if _, depleted := motion.DepletedBy(); depleted {
				t.Fatalf("the vehicle ran out with a usable reserve in %s", check.reserve)
			}
			if left := remaining(moved, check.first); left != 0 {
				t.Errorf("%s holds %d after the minute it lasts", check.first, left)
			}
			if lost := spent(moved, start, check.reserve); lost <= 0 {
				t.Errorf("the reserve spent %d over the rest of the window", lost)
			}
			if len(motion.Exhausted) != 1 || motion.Exhausted[0] != check.first {
				t.Errorf("the journey used up %v, not just %s", motion.Exhausted, check.first)
			}
			if moved.Holding()[0] != 0 {
				t.Error("the vehicle is not moving on the source that still holds something")
			}
		})
	}
}

// A vehicle reaches its reserve when one source ends, whether it ends inside a window or exactly with
// one. A source that ends exactly with a window leaves nothing of that window to the next source, and
// the vehicle carries on in the window after it: reading the end of a window as the end of the journey
// stopped a vehicle with a full tank beside it.
func TestASourceEndingWithAWindowHandsOverToTheReserve(t *testing.T) {
	start := vehicle(fleet.PowertrainHybrid, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery:  lasting(fleet.SourceBattery, time.Minute),
		fleet.SourceGasoline: capacity[fleet.SourceGasoline],
	})
	first, after, err := start.Journey(driving(time.Minute), epoch.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, depleted := first.DepletedBy(); depleted {
		t.Fatal("a vehicle that ended its battery exactly with a window was reported as run out")
	}

	later, moved, err := after.Journey([]simulation.Span{
		{Mode: simulation.Driving, From: epoch.Add(time.Minute), To: epoch.Add(2 * time.Minute)},
	}, epoch.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, depleted := later.DepletedBy(); depleted {
		t.Fatal("the window after the switch reported a vehicle with a usable reserve as run out")
	}
	if lost := spent(moved, after, fleet.SourceGasoline); lost <= 0 {
		t.Errorf("the reserve spent %d in the window after the battery ended", lost)
	}
}

func TestTheUnusedPartOfTheWindowContinuesOnTheReserve(t *testing.T) {
	// spend five minutes of the reserve on top of it: none of the window is lost at the switch.
	start := vehicle(fleet.PowertrainHybrid, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery:  capacity[fleet.SourceBattery] / 60,
		fleet.SourceGasoline: capacity[fleet.SourceGasoline],
	})
	_, switched, err := start.Journey(driving(6*time.Minute), epoch.Add(6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	// The same five minutes driven on a reserve nothing else competes with, so the two spendings of
	// the tank are the same five minutes of driving.
	reference := vehicle(fleet.PowertrainHybrid, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery:  0,
		fleet.SourceGasoline: capacity[fleet.SourceGasoline],
	})
	_, fiveMinutes, err := reference.Journey(driving(5*time.Minute), epoch.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := spent(switched, start, fleet.SourceGasoline),
		spent(fiveMinutes, reference, fleet.SourceGasoline); got != want {
		t.Errorf("the reserve spent %d over the rest of the window; five minutes cost %d", got, want)
	}
}

func TestAnEmptyBatteryBesideAFullTankIsNotDepletion(t *testing.T) {
	for _, powertrain := range []fleet.PowertrainType{fleet.PowertrainHybrid, fleet.PowertrainGas} {
		t.Run(string(powertrain), func(t *testing.T) {
			start := vehicle(powertrain, map[fleet.SourceKind]fleet.Amount{
				fleet.SourceBattery:  0,
				fleet.SourceLPG:      0,
				fleet.SourceGasoline: capacity[fleet.SourceGasoline],
			})
			motion, moved, err := start.Journey(driving(time.Minute), epoch.Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if _, depleted := motion.DepletedBy(); depleted {
				t.Error("a vehicle beside a full tank is reported as depleted")
			}
			if lost := spent(moved, start, fleet.SourceGasoline); lost <= 0 {
				t.Error("the journey spent nothing of the tank")
			}
		})
	}
}

// A journey is cut into windows a caller names, and a caller's boundaries are whatever the clock says:
// the same journey is checked whole, cut into a few long windows, and cut into thousands of short ones.
func TestSplittingAJourneyChangesNothing(t *testing.T) {
	for _, check := range []struct {
		wholeWindow time.Duration
		pieces      []int
	}{
		// A window the model can cut into whole nanoseconds, so every boundary falls where the reserve
		// ends as well.
		{wholeWindow: 4 * time.Minute, pieces: []int{1, 5, 600, 5000}},
		// A window it cannot: a seventh of four minutes is not a whole number of nanoseconds, and the
		// pieces a caller names are the pieces it is given.
		{wholeWindow: 4 * time.Minute, pieces: []int{7}},
	} {
		for _, pieces := range check.pieces {
			t.Run(fmt.Sprintf("%d pieces of %v", pieces, check.wholeWindow), func(t *testing.T) {
				// Both magnitudes are checked: a journey split into pieces longer than the reserve
				// lasts, and one split into thousands of pieces that each end while it still holds
				// something.
				start := vehicle(fleet.PowertrainHybrid, map[fleet.SourceKind]fleet.Amount{
					fleet.SourceBattery:  capacity[fleet.SourceBattery] / 20,
					fleet.SourceGasoline: capacity[fleet.SourceGasoline] / 30,
				})
				one, _, err := start.Journey(driving(check.wholeWindow), epoch.Add(check.wholeWindow))
				if err != nil {
					t.Fatal(err)
				}

				state := start
				spans := equalSpans(pieces, check.wholeWindow)
				for _, span := range spans {
					_, state, err = state.Journey([]simulation.Span{span}, span.To)
					if err != nil {
						t.Fatal(err)
					}
				}
				played, _, err := start.Journey(spans, epoch.Add(check.wholeWindow))
				if err != nil {
					t.Fatal(err)
				}
				if got, want := depletedAt(played), depletedAt(one); !got.Equal(want) {
					t.Errorf("%d pieces ran out at %v, one window at %v", pieces, got, want)
				}
				if got, want := played.Position, one.Position; !samePlace(got, want) {
					t.Errorf("%d pieces in one call ended at %v, one window at %v", pieces, got, want)
				}
				if got, want := state.Position, one.Position; !samePlace(got, want) {
					t.Errorf("%d separate ticks ended at %v, one window at %v", pieces, got, want)
				}
				if got, want := state.Holding(), one.Holding; !sameAmounts(got, want) {
					t.Errorf("%d separate ticks left %v, one window left %v", pieces, got, want)
				}
			})
		}
	}
}

func sameAmounts(left, right []fleet.Amount) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// samePlace reports whether two positions are the same place to within a few metres. Every count the
// model keeps is whole, but a position is a pair of coordinates, so the last digits of two readings of
// the same place may differ by less than the width of a lane.
func samePlace(left, right fleet.Position) bool {
	const metresPerDegree = 111_190
	across := (left.Longitude - right.Longitude) * metresPerDegree * 0.733
	up := (left.Latitude - right.Latitude) * metresPerDegree
	return across*across+up*up < metresOfTolerance*metresOfTolerance
}

// metresOfTolerance is how far apart two readings of one place may be. It is far below the distance a
// vehicle covers between two ticks and far above the precision of a coordinate.
const metresOfTolerance = 50

// equalSpans cuts one window into equal pieces, which is what a sequence of regular ticks produces.
func equalSpans(pieces int, whole time.Duration) []simulation.Span {
	spans := make([]simulation.Span, 0, pieces)
	for piece := 0; piece < pieces; piece++ {
		spans = append(spans, simulation.Span{
			Mode: simulation.Driving,
			From: epoch.Add(time.Duration(piece) * whole / time.Duration(pieces)),
			To:   epoch.Add(time.Duration(piece+1) * whole / time.Duration(pieces)),
		})
	}
	return spans
}

func TestDepletionIsTheFirstMicrosecondWithoutReserve(t *testing.T) {
	// One minute of driving empties this battery exactly.
	start := vehicle(fleet.PowertrainElectric, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery: lasting(fleet.SourceBattery, time.Minute),
	})
	at, state, err := start.Journey(driving(2*time.Minute), epoch.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := depletedAt(at), epoch.Add(time.Minute); !got.Equal(want) {
		t.Errorf("the vehicle ran out at %v, the arithmetic says %v", got, want)
	}

	// The last microsecond before the reserve ends still covers the window that reaches it.
	edge := vehicle(fleet.PowertrainElectric, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery: 1_000_000,
	})
	edge.ProcessedAt = epoch.Add(time.Minute - time.Microsecond)
	before, _, err := edge.Journey([]simulation.Span{
		{Mode: simulation.Driving, From: edge.ProcessedAt, To: epoch.Add(time.Minute)},
	}, epoch.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, depleted := before.DepletedBy(); depleted {
		t.Error("a source holding a whole millionth was reported as spent a microsecond early")
	}

	// A tick that arrives after the vehicle has run out spends nothing more and still reports the
	// moment it was asked about, so the next tick is not a journey into the past.
	later, moved, err := state.Journey([]simulation.Span{
		{Mode: simulation.Driving, From: state.ProcessedAt, To: epoch.Add(time.Hour)},
	}, epoch.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, depleted := later.DepletedBy(); depleted {
		t.Error("a vehicle that had already run out reported running out a second time")
	}
	if got := moved.ProcessedAt; !got.Equal(epoch.Add(time.Hour)) {
		t.Errorf("the state stayed at %v instead of the moment it was asked about", got)
	}
}

func TestTimeIsNotPlayedBackwards(t *testing.T) {
	start := vehicle(fleet.PowertrainElectric, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery: capacity[fleet.SourceBattery],
	})
	_, _, err := start.Journey([]simulation.Span{
		{Mode: simulation.Driving, From: epoch, To: epoch.Add(-time.Second)},
	}, epoch.Add(-time.Second))
	if !errors.Is(err, simulation.ErrTimeReversed) {
		t.Fatalf("a journey ending before the processed moment answered %v", err)
	}
}

func TestStateIsRefusedRatherThanGuessedAt(t *testing.T) {
	start := vehicle(fleet.PowertrainGas, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceLPG:      capacity[fleet.SourceLPG],
		fleet.SourceGasoline: capacity[fleet.SourceGasoline],
	})
	missing := start
	missing.Sources[0].Charged = nil
	if _, _, err := missing.Journey(driving(time.Minute), epoch.Add(time.Minute)); !errors.Is(
		err, simulation.ErrStateUnusable) {
		t.Fatalf("a source with no charge of its own answered %v", err)
	}

	unrouted := start
	unrouted.RouteID = "no-such-route"
	if _, _, err := unrouted.Journey(driving(time.Minute), epoch.Add(time.Minute)); !errors.Is(
		err, simulation.ErrStateUnusable) {
		t.Fatalf("a route this build does not declare answered %v", err)
	}
}

func TestJourneySpansMustMeetEndToEnd(t *testing.T) {
	start := vehicle(fleet.PowertrainElectric, map[fleet.SourceKind]fleet.Amount{
		fleet.SourceBattery: capacity[fleet.SourceBattery],
	})
	gap := []simulation.Span{
		{Mode: simulation.Driving, From: epoch, To: epoch.Add(time.Minute)},
		{Mode: simulation.Paused, From: epoch.Add(2 * time.Minute), To: epoch.Add(3 * time.Minute)},
	}
	if _, _, err := start.Journey(gap, epoch.Add(3*time.Minute)); !errors.Is(
		err, simulation.ErrTimeReversed) {
		t.Fatalf("spans that do not meet answered %v", err)
	}
}
