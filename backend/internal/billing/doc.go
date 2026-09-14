// Package billing turns the time a ride spent in each mode into the amount it has cost. It computes
// from durations and a price list alone, so it has no side effects: it opens no transaction, sends
// nothing, reads no environment and writes no state. Everything it refuses — a negative duration, a
// rate nobody could set, a value the signed 64-bit range cannot hold — comes back as an error rather
// than as a number a caller could mistake for an answer.
package billing
