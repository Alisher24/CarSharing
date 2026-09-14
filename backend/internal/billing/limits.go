package billing

import (
	"math"
	"math/bits"
)

// MicrosecondsPerMinute is the unit the billing policy is stated in. Durations are whole
// microseconds, and a begun minute is sixty million of them.
const MicrosecondsPerMinute int64 = 60_000_000

// maxRepresentable is the largest value of the signed 64-bit range, which is the range a duration, an
// amount and the contract's whole numbers all state.
const maxRepresentable = math.MaxInt64

// SumMicroseconds adds the microseconds of one mode and reports whether their total left the signed
// 64-bit range, which is the range the contract publishes a duration in. The counts are added rather
// than a duration, because a duration measured in nanoseconds cannot hold a count of microseconds this
// large at all: the sum is the smaller number, so the range is checked on the sum itself. A count
// cannot be negative, so a total below the first summand is one that wrapped.
func SumMicroseconds(left, right int64) (int64, error) {
	if left < 0 || right < 0 {
		return 0, Refusal{Reason: "a duration cannot be negative"}
	}
	total := left + right
	if total < left {
		return 0, Refusal{Reason: "the duration of a mode does not fit the signed 64-bit range"}
	}
	return total, nil
}

// times multiplies two values that are not negative and reports whether the product left the signed
// 64-bit range, together with the digits it leaves there. The range is checked against the wider
// unsigned product rather than after the multiplication, where the answer would already be wrapped.
func times(left int64, right int64) (int64, bool) {
	if left < 0 || right < 0 {
		return 0, true
	}
	high, low := bits.Mul64(uint64(left), uint64(right))
	if high != 0 || low > uint64(maxRepresentable) {
		return int64(low), true
	}
	return int64(low), false
}

// plus adds two values that are not negative and reports whether the sum left the signed 64-bit range.
func plus(left int64, right int64) (int64, bool) {
	if left < 0 || right < 0 {
		return 0, true
	}
	if left > maxRepresentable-right {
		return 0, true
	}
	return left + right, false
}
