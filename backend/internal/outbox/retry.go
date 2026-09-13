package outbox

import (
	"math/rand/v2"
	"time"
)

// The retry policy of one task: the first delay, the delay no attempt waits longer than, and how much
// of a delay the random part may add. The spread keeps two tasks that failed together from being
// retried together for as long as they keep failing, without making either wait noticeably longer.
const (
	firstRetryDelay = time.Second
	maxRetryDelay   = 60 * time.Second
	spreadDivisor   = 5
)

// retryDelay is how long the task waits after its nth failed attempt. The delay doubles from the
// first attempt, stops at the maximum, and carries a random part of at most a fifth of itself.
func retryDelay(attempts int) time.Duration {
	delay := firstRetryDelay
	for range attempts - 1 {
		if delay >= maxRetryDelay {
			break
		}
		delay *= 2
	}
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	return delay + rand.N(delay/spreadDivisor+1)
}
