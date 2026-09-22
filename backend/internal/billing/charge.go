package billing

import "time"

// PolicyPerModeStartedMinuteV1 identifies the rule this package applies: the whole ride is summed per
// mode, each sum is rounded up to begun minutes once, and every begun minute of a mode costs that
// mode's rate. Switching mode does not cost anything by itself. The rule is declared once here; the
// stored price list, the demonstration catalog and the published snapshot name this declaration
// instead of spelling the identifier again.
const PolicyPerModeStartedMinuteV1 = "per_mode_started_minute_v1"

// RateCount is how many rates a price list states, one per mode. The product of all rates is what a
// charge in tyiyn is made of, so the count is stated once rather than implied by the two fields.
const RateCount = 2

// Rates are the prices a charge is computed at, in whole tyiyn per begun minute of each mode. They
// are the rates frozen into a rental when it was reserved, so a change of the catalog cannot reach a
// ride that already exists.
type Rates struct {
	Driving RateTyiynPerStartedMinute
	Paused  RateTyiynPerStartedMinute
}

// Refusal reports why a charge could not be computed: a duration or a rate that cannot be one, or a
// value beyond the signed 64-bit range. The amount a refusal carries is the zero Charge, so no
// partial answer can be read as a result.
//
// The rules below return one of the reasons declared under this type, because an invoice refuses a
// draft under the same rules and states the same reason.
type Refusal struct {
	Reason string
}

func (r Refusal) Error() string { return r.Reason }

var (
	// RefusalNegativeDuration reports a duration that is less than no time.
	RefusalNegativeDuration = Refusal{Reason: "a duration cannot be negative"}

	// RefusalNegativeMinutes reports a count of minutes begun that is less than none.
	RefusalNegativeMinutes = Refusal{Reason: "minutes begun cannot be negative"}

	// RefusalNegativeRate reports a rate that is less than nothing per begun minute.
	RefusalNegativeRate = Refusal{Reason: "a rate cannot be negative"}

	// RefusalModeAmountBeyondRange reports the amount of one mode leaving the signed 64-bit range.
	RefusalModeAmountBeyondRange = Refusal{
		Reason: "the amount of a mode does not fit the signed 64-bit range",
	}

	// RefusalTotalAmountBeyondRange reports a total leaving the signed 64-bit range.
	RefusalTotalAmountBeyondRange = Refusal{
		Reason: "the total amount does not fit the signed 64-bit range",
	}

	// RefusalModeDurationBeyondRange reports the duration of one mode leaving the signed 64-bit range.
	RefusalModeDurationBeyondRange = Refusal{
		Reason: "the duration of a mode does not fit the signed 64-bit range",
	}
)

// Charge is what a ride owes at one moment under one price list: how long it spent in each mode, the
// minutes begun in each, what those minutes cost, and the rates they were priced at. It is the whole
// of what an invoice states about a ride, so an invoice is written from a charge and from nothing
// else.
type Charge struct {
	Rates Rates

	DrivingDuration time.Duration
	PausedDuration  time.Duration
	DrivingMinutes  int64
	PausedMinutes   int64

	// DrivingAmount and PausedAmount are what each mode contributed. They are carried rather than
	// recomputed from the minutes, so the total of an invoice is the sum of the amounts it publishes
	// by construction instead of by a second multiplication that could disagree with the first.
	DrivingAmount AmountTyiyn
	PausedAmount  AmountTyiyn

	TotalTyiyn AmountTyiyn
}

// StartedMinutes is the rounding rule: for a duration of d microseconds the ride has begun
// d/minute + (d % minute != 0 ? 1 : 0) minutes, and a duration that is not negative has begun at
// least none. It is defined for one mode summed over the whole ride: applying it to each interval
// separately would bill three twenty-second intervals as three begun minutes instead of one.
func StartedMinutes(duration time.Duration) (int64, error) {
	if duration < 0 {
		return 0, RefusalNegativeDuration
	}
	microseconds := duration.Microseconds()
	minutes := microseconds / MicrosecondsPerMinute
	if microseconds%MicrosecondsPerMinute != 0 {
		minutes++
	}
	return minutes, nil
}

// PricedAt prices the minutes begun in one mode at that mode's rate. A negative rate is a price list
// the operator could not have set, and a rate that cannot pay every minute begun is refused before the
// product is computed rather than answered with the digits that overflow leaves behind.
func PricedAt(rate RateTyiynPerStartedMinute, minutes int64) (AmountTyiyn, error) {
	if rate < 0 {
		return 0, RefusalNegativeRate
	}
	if minutes < 0 {
		return 0, RefusalNegativeMinutes
	}
	product, carried := times(int64(rate), minutes)
	if carried {
		return 0, RefusalModeAmountBeyondRange
	}
	return AmountTyiyn(product), nil
}

// Compute is the whole policy in one call: the minutes begun in each mode, each of them priced at its
// own rate, and the two amounts added together.
func Compute(rates Rates, driving, paused time.Duration) (Charge, error) {
	drivingMinutes, err := StartedMinutes(driving)
	if err != nil {
		return Charge{}, err
	}
	pausedMinutes, err := StartedMinutes(paused)
	if err != nil {
		return Charge{}, err
	}
	drivingAmount, err := PricedAt(rates.Driving, drivingMinutes)
	if err != nil {
		return Charge{}, err
	}
	pausedAmount, err := PricedAt(rates.Paused, pausedMinutes)
	if err != nil {
		return Charge{}, err
	}
	total, err := Add(drivingAmount, pausedAmount)
	if err != nil {
		return Charge{}, err
	}
	return Charge{
		Rates:           rates,
		DrivingDuration: driving,
		PausedDuration:  paused,
		DrivingMinutes:  drivingMinutes,
		PausedMinutes:   pausedMinutes,
		DrivingAmount:   drivingAmount,
		PausedAmount:    pausedAmount,
		TotalTyiyn:      total,
	}, nil
}

// Add totals two amounts. The total is checked as well as each product: two amounts that each fit can
// still add up to one that does not.
func Add(left, right AmountTyiyn) (AmountTyiyn, error) {
	total, carried := plus(int64(left), int64(right))
	if carried {
		return 0, RefusalTotalAmountBeyondRange
	}
	return AmountTyiyn(total), nil
}
