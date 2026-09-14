package billing

// AmountTyiyn is a sum of money in whole tyiyn, which is the unit every amount of this program is
// counted in. It is declared as its own type so that an amount cannot be handed to something expecting
// a duration, a rate or a version: the four are all whole numbers and none of them may be mistaken for
// another.
type AmountTyiyn int64

// RateTyiynPerStartedMinute is what one begun minute of a mode costs. Rates of the two modes stand
// together in Rates, so a charge is computed at a price list rather than at whichever rate a caller
// happened to name.
type RateTyiynPerStartedMinute int64

// Microseconds is a duration as the contract publishes it: whole microseconds. It is a count rather
// than a duration, because a duration measured in nanoseconds cannot hold a count of microseconds this
// large at all.
type Microseconds int64
