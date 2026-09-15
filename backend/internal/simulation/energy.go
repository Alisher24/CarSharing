package simulation

import (
	"math/big"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// nanosecond is one unit of the time every rule here is stated in, and the resolution a moment is
// counted in. Counting a charge in whole microseconds instead would drop the part of one at every
// boundary a caller names, and a part dropped at every tick makes the same journey cost a little less
// when it is played in pieces than when it is played in one call.
const nanosecond = time.Nanosecond

// DrivingMinutesPerSource is how long a whole source lasts while the vehicle moves. The rate of every
// source follows from its own capacity and this duration, so no absolute consumption is written down a
// second time beside the capacity it belongs to.
const DrivingMinutesPerSource = 60

// PausedFractionOfDriving is what standing still spends of what moving spends. Standing still is not
// free in a demonstration: a paused ride has to be able to run its reserve down, or a vehicle could
// wait for ever without ever reaching depletion.
const (
	pausedFractionNumerator   = 1
	pausedFractionDenominator = 10
)

// periodOfDriving is the window a rate is stated over: the nanoseconds a whole source lasts. Every
// charge is counted in millionths multiplied by this one window, in either mode, so a charge means the
// same thing whether the vehicle was moving or standing when it was made.
const periodOfDriving = int64(DrivingMinutesPerSource) * int64(time.Minute/nanosecond)

// Source is one inventory a vehicle carries. What it holds is not stored beside the capacity: a reserve
// is a rounded number, and one rounded away once per tick would make the same journey cost different
// amounts depending on how often the model was asked to move. The charge is the number that moves
// forward, and the reserve follows from it.
type Source struct {
	Kind     fleet.SourceKind
	Capacity fleet.Amount

	// Rate is how the source is spent while the vehicle moves. Paused consumption is the same fraction
	// of it, so the two can never disagree about how thirsty a source is.
	Rate Rate

	// Charged is the sum of what every window has asked of the source since the ride began, in
	// millionths of its unit multiplied by the rate's period. Keeping the sum rather than the part of a
	// millionth left over is what makes a journey split into many short steps cost exactly what the same
	// journey costs in one call: the sum depends on the time the windows cover and on nothing else, so
	// no part of a millionth is lost to the order they arrive in.
	//
	// It is held by pointer because a sum grows as the ride goes on, and a source copied by value would
	// carry the same sum as the copy it came from rather than one of its own.
	Charged *big.Int
}

// Rate is how fast a source is spent, as a fraction: this many millionths of its unit over this many
// nanoseconds. Keeping the fraction rather than a rounded rate is what makes a whole source last
// exactly as long as the rule says instead of running a little short.
type Rate struct {
	units  int64
	period int64
}

// RateOf is the driving rate of a source whose full capacity is this.
func RateOf(capacity fleet.Amount) Rate {
	return Rate{units: int64(capacity), period: periodOfDriving}
}

// DrivingRate is what the source gives over one period of movement.
func (s Source) DrivingRate() Rate { return s.Rate }

// PausedRate is what the source gives over one period of standing still. The period is stretched rather
// than the rate rounded down, so that a tenth of a source's worth of movement is exactly a tenth: a
// rate divided into whole millionths first would lose the part of one and end the source early.
func (s Source) PausedRate() Rate {
	return Rate{
		units:  s.Rate.units * pausedFractionNumerator,
		period: s.Rate.period * pausedFractionDenominator,
	}
}

// SourceWith is one inventory of a vehicle as a stored reserve describes it: how much a full one holds
// and how much is left of it now. It is how the model is handed a vehicle to move.
func SourceWith(kind fleet.SourceKind, capacity, remaining fleet.Amount) Source {
	source := Source{Kind: kind, Capacity: capacity, Rate: RateOf(capacity), Charged: big.NewInt(0)}
	source.Charged = source.charge(remaining)
	return source
}

// Remaining is what is left of the source: what it holds when full, less the whole millionths its
// windows have charged. The part of a millionth the charge has reached but not completed is not a
// reserve a caller can act on, so it is not reported; a source that has run out reports nothing, and
// one that has not always reports something.
func (s Source) Remaining() fleet.Amount {
	if s.exhausted() {
		return 0
	}
	spent := new(big.Int).Quo(s.Charged, big.NewInt(s.Rate.period)).Int64()
	return s.Capacity - fleet.Amount(spent)
}

// exhausted reports whether the source has spent everything it held, which is what a charge reaching
// the whole of what the source is worth means. The comparison is made in the unit the charge is kept in
// rather than in a reserve rounded to whole millionths: rounding first left a part of a millionth
// behind on every window, and a part left behind on every window is a part the next one spends again.
func (s Source) exhausted() bool {
	return s.Charged.Cmp(s.budget()) >= 0
}

// budget is the whole of what the source is worth in the unit the charge is kept in: its capacity,
// stated in millionths a period, over as many periods as the rate is stated in.
func (s Source) budget() *big.Int {
	return new(big.Int).Mul(big.NewInt(int64(s.Rate.units)), big.NewInt(s.Rate.period))
}

// charge is the sum a reserve read from storage stands for: nothing has been charged yet, so the
// reserve is the whole of what the source holds at the moment the ride begins.
func (s Source) charge(remaining fleet.Amount) *big.Int {
	spent := int64(s.Capacity) - int64(remaining)
	return new(big.Int).Mul(big.NewInt(spent), big.NewInt(s.Rate.period))
}

// consume charges one window of this source for the time it covers. The charge of a long window is a
// product of two large counts, so the sum is held exactly.
func (s Source) consume(rate Rate, duration time.Duration, charged *big.Int) {
	window := new(big.Int).Mul(big.NewInt(rate.units), big.NewInt(duration.Nanoseconds()))
	charged.Add(charged, window)
}

// spentAt is the moment inside a window at which this source has nothing left to give. It reports the
// moment the source runs out, which is the end of the window when it covers the whole of it.
//
// The moment is the first nanosecond at which the window would ask for more than the source still
// holds, and what it holds is read from the charge rather than from a reserve rounded to whole
// millionths: rounding first left a part of a millionth behind on every window, and a part left behind
// on every window is a part the next one charges again, which is how a journey split into pieces came
// to spend less of a reserve than the same journey played in one call.
func (s Source) spentAt(rate Rate, charged *big.Int, from time.Time, window time.Duration) time.Time {
	covering := from.Add(window)
	if rate.units <= 0 || rate.period <= 0 || s.exhausted() {
		return covering
	}
	gives := s.wholeNanosecondsLeft(rate, charged)
	if gives > window.Nanoseconds() {
		return covering
	}
	return from.Add(time.Duration(gives) * nanosecond)
}

// wholeNanosecondsLeft is how many whole nanoseconds of this window the source can still be charged
// for, counted from the start of the window. A source that has given everything answers nothing.
//
// What the source is worth is stated over the period of the rate it is spent at, which is what makes a
// paused vehicle last ten times as long on the same reserve without the rate ever being rounded.
func (s Source) wholeNanosecondsLeft(rate Rate, charged *big.Int) int64 {
	held := new(big.Int).Mul(big.NewInt(rate.units), big.NewInt(rate.period))
	held.Sub(held, charged)
	needed := new(big.Int).Add(held, big.NewInt(rate.units-1))
	return needed.Quo(needed, big.NewInt(rate.units)).Int64()
}
