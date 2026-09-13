// Package idempotency remembers the result of one mutating command, so that a client whose answer
// was lost can repeat the command and be told what the first attempt decided instead of causing the
// change twice. A result is scoped to the account that sent the command and is written inside the
// transaction that makes the change: a visible row always carries an answer, and a change that is
// rolled back leaves no row behind.
package idempotency
