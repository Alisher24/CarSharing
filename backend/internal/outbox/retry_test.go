package outbox

import (
	"testing"
	"time"
)

// The retry policy of one task: the delay doubles from the first attempt, stops at the maximum, and
// carries a random part of at most a fifth of itself.
func TestRetryDelayGrowsFromOneSecondToTheMaximum(t *testing.T) {
	for _, expected := range []struct {
		attempts int
		base     time.Duration
	}{
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 32 * time.Second},
		{7, maxRetryDelay},
		{8, maxRetryDelay},
		{problemAttempts, maxRetryDelay},
		{problemAttempts * 100, maxRetryDelay},
	} {
		delay := retryDelay(expected.attempts)
		spread := expected.base / spreadDivisor
		if delay < expected.base || delay > expected.base+spread {
			t.Errorf("attempt %d waits %s, want between %s and %s",
				expected.attempts, delay, expected.base, expected.base+spread)
		}
	}
}

// A delay that carried no spread would retry two tasks that failed together at the same moment for
// as long as they keep failing, which is what the random part is there to break up.
func TestRetryDelaySpreadsRepeatedAttempts(t *testing.T) {
	delays := map[time.Duration]bool{}
	for range 50 {
		delays[retryDelay(problemAttempts)] = true
	}
	if len(delays) == 1 {
		t.Fatal("every attempt waits exactly the same time")
	}
}
