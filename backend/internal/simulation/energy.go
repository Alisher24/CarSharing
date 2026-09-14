package simulation

import (
	"math/big"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// microsecond is one unit of the time every rule here is stated in.
const microsecond = time.Microsecond

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

// periodOfDriving is the window a rate is stated over: the microseconds a whole source lasts. Every
// charge is counted in millionths multiplied by this one window, in either mode, so a charge means the
// same thing whether the vehicle was moving or standing when it was made.
const periodOfDriving = int64(DrivingMinutesPerSource) * int64(time.Minute/time.Microsecond)

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
// microseconds. Keeping the fraction rather than a rounded rate is what makes a whole source last
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
// windows have charged. It is never below nothing, because a ride that runs out stops spending.
func (s Source) Remaining() fleet.Amount {
	spent := new(big.Int).Quo(s.Charged, big.NewInt(s.Rate.period)).Int64()
	if spent >= int64(s.Capacity) {
		return 0
	}
	return s.Capacity - fleet.Amount(spent)
}

// charge is the sum a reserve read from storage stands for: nothing has been charged yet, so the
// reserve is the whole of what the source holds at the moment the ride begins.
func (s Source) charge(remaining fleet.Amount) *big.Int {
	spent := int64(s.Capacity) - int64(remaining)
	return new(big.Int).Mul(big.NewInt(spent), big.NewInt(s.Rate.period))
}

// consume charges one window of this source. A window costs what the rate says over the time it covers,
// and only whole millionths are charged: the part of one is never subtracted, so it is not lost either.
//
// The charge of a long window is a product of two large counts, so the sum is held exactly.
func (s Source) consume(rate Rate, duration time.Duration, charged *big.Int) {
	window := new(big.Int).Mul(big.NewInt(rate.units), big.NewInt(int64(duration/microsecond)))
	charged.Add(charged, window)
}

// spentAt is the moment inside a window at which this source has nothing left to give. It reports the
// moment the source runs out, which is the end of the window when it covers the whole of it.
//
// A source lasts what it still holds plus whatever of it the windows have spent, and a window asks for
// what the rate gives over it, so the moment is the first microsecond at which the charge would pass
// that. Both sides are products of a rate and a window, held exactly rather than as a rate rounded to a
// whole millionth a microsecond: the division is the moment a ride's ending and its invoice are both
// read from.
func (s Source) spentAt(rate Rate, charged *big.Int, from time.Time, window time.Duration) time.Time {
	covering := from.Add(window)
	remaining := s.Remaining()
	if remaining <= 0 || rate.units <= 0 || rate.period <= 0 {
		return covering
	}
	held := new(big.Int).Mul(big.NewInt(int64(remaining)), big.NewInt(rate.period))
	held.Add(held, charged)
	// The first whole microsecond that asks for more than the source holds.
	needed := new(big.Int).Add(held, big.NewInt(rate.units-1))
	needed.Quo(needed, big.NewInt(rate.units))
	if needed.Sign() <= 0 {
		return from
	}
	if needed.Cmp(big.NewInt(window.Microseconds())) > 0 {
		return covering
	}
	return from.Add(time.Duration(needed.Int64()) * microsecond)
}
