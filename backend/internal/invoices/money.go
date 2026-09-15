package invoices

import (
	"strconv"
	"strings"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
)

// How an amount is written for a person: the som a sum of tyiyn makes, the two minor digits below it,
// and the unit the interface names. The rule is the one the browser writes a price by
// (`frontend/src/features/fleet/money.ts`), stated here because a process cannot import it.
const (
	// tyiynInSom is how many tyiyn make one som.
	tyiynInSom = 100

	// moneySuffix is what follows an amount, in the unit the interface writes it in: an amount is
	// written as `12,34 сома` and never as a bare number.
	moneySuffix = " сома"

	// minorDigitsCount is how many digits below the som are always printed, so an amount is written
	// as `12,00` rather than as `12`.
	minorDigitsCount = 2

	// digitGroup is how many digits stand between two group separators.
	digitGroup = 3

	// groupSeparator is what separates the groups. The browser locale inserts a non-breaking space
	// where this letter writes an ordinary one, which is the one difference between the two: the
	// letter is plain text a person reads in a terminal or a mailbox, and no reader of it separates
	// the groups again.
	groupSeparator = ' '
)

// somText renders a sum of whole tyiyn the way the interface writes money: the som it makes, a comma
// and the two minor digits, followed by the unit. The digits are computed on whole numbers, so a sum
// beyond the exact range of a floating-point number arrives digit for digit.
func somText(amount billing.AmountTyiyn) string {
	whole := int64(amount) / tyiynInSom
	return groupedDigits(whole) + "," + minorDigits(int64(amount)%tyiynInSom) + moneySuffix
}

// groupedDigits writes a whole number of som, grouping its digits from the right.
func groupedDigits(value int64) string {
	digits := strconv.FormatInt(value, 10)
	var grouped strings.Builder
	for position, digit := range digits {
		if position > 0 && (len(digits)-position)%digitGroup == 0 {
			grouped.WriteRune(groupSeparator)
		}
		grouped.WriteRune(digit)
	}
	return grouped.String()
}

// minorDigits writes the digits below one som, which are printed even when they are zero.
func minorDigits(value int64) string {
	minor := strconv.FormatInt(value, 10)
	for len(minor) < minorDigitsCount {
		minor = "0" + minor
	}
	return minor
}
