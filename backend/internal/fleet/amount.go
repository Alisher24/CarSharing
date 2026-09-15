package fleet

import (
	"errors"
	"math"
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

// ErrAmountUnreadable reports a decimal this package cannot read as an amount: one that is not a
// canonical number, or one whose digits ask for more precision than an amount counts in.
var ErrAmountUnreadable = errors.New("the decimal does not state an amount")

// ParseAmount reads an amount the way the contract spells one. Six fractional digits is all an amount
// counts in, so a decimal that states more of them is refused rather than rounded: a reserve nobody
// can store is a reserve the caller meant something else by.
func ParseAmount(decimal string) (Amount, error) {
	whole, fraction, split := strings.Cut(decimal, ".")
	if whole == "" || !isDigits(whole) {
		return 0, ErrAmountUnreadable
	}
	if split && (fraction == "" || len(fraction) > amountFractionWidth || !isDigits(fraction)) {
		return 0, ErrAmountUnreadable
	}
	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, ErrAmountUnreadable
	}
	padded := fraction + strings.Repeat("0", amountFractionWidth-len(fraction))
	fractionUnits, err := strconv.ParseInt(padded, 10, 64)
	if err != nil {
		return 0, ErrAmountUnreadable
	}
	if units > (math.MaxInt64-fractionUnits)/AmountScale {
		return 0, ErrAmountUnreadable
	}
	return Amount(units*AmountScale + fractionUnits), nil
}

func isDigits(text string) bool {
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
