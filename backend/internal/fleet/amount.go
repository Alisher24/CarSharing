package fleet

import (
	"strconv"
	"strings"
)

// AmountScale is how many stored units make one whole unit of measure. Six fractional digits is
// both the precision the contract's decimal admits and the scale the database column stores, so an
// inventory crosses neither boundary as a rounded value.
const AmountScale = 1_000_000

// amountFractionWidth is how many digits the fractional part is padded to before its trailing
// zeros are dropped.
const amountFractionWidth = 6

// Amount is an energy inventory counted in millionths of its unit. Counting in whole millionths
// keeps a comparison against a threshold exact, which a binary floating-point remainder would not.
type Amount int64

// Decimal renders the amount the way the contract spells one: canonical, at most six fractional
// digits, and never a trailing fractional zero.
func (a Amount) Decimal() string {
	whole := strconv.FormatInt(int64(a)/AmountScale, 10)
	fraction := int64(a) % AmountScale
	if fraction == 0 {
		return whole
	}
	padded := strconv.FormatInt(fraction, 10)
	padded = strings.Repeat("0", amountFractionWidth-len(padded)) + padded
	return whole + "." + strings.TrimRight(padded, "0")
}
