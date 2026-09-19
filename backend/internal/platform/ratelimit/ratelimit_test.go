package ratelimit

import (
	"testing"
	"time"
)

// The budget is the limit itself: a window grants the attempts its limit names and refuses the one
// after them. The count a claim is decided against is how many attempts the window had already
// decided, so the attempt the limit names is decided against one less than the limit and the attempt
// after it against the limit itself. A limit of one is the case that shows the count is read from
// zero: it grants one attempt rather than refusing every attempt the subject ever makes, and a window
// the statement has just replaced is decided against zero like the first attempt of any window.
func TestAnAttemptFitsWhileFewerThanTheLimitHaveBeenDecided(t *testing.T) {
	for _, asked := range []struct {
		name    string
		decided int
		limit   int
		granted bool
	}{
		{name: "nothing decided yet", decided: 0, limit: 10, granted: true},
		{name: "the attempt the limit names", decided: 9, limit: 10, granted: true},
		{name: "the attempt after the limit", decided: 10, limit: 10},
		{name: "a count the statement left at the limit", decided: 11, limit: 10},
		{name: "the one attempt a limit of one names", decided: 0, limit: 1, granted: true},
		{name: "the attempt after a limit of one", decided: 1, limit: 1},
		{
			// A window the statement replaced is decided against a count of zero, exactly as the first
			// attempt of a window is: returning access is the statement's doing, and this is what it hands
			// the decision once it has done it.
			name: "the first attempt of a window the statement replaced", decided: 0, limit: 10, granted: true,
		},
	} {
		t.Run(asked.name, func(t *testing.T) {
			granted := withinBudget(asked.decided, Limit{Attempts: asked.limit})
			if granted != asked.granted {
				t.Fatalf("a count of %d against a limit of %d was answered granted=%v, want %v",
					asked.decided, asked.limit, granted, asked.granted)
			}
		})
	}
}

// Every wait this package reports is floored, which is what keeps a window that has already ended from
// advertising a rounded-down zero: a caller told to wait no time at all comes straight back.
func TestAReportedWaitIsNeverShorterThanTheMinimum(t *testing.T) {
	if ended := remaining(time.Now().Add(-time.Hour), time.Minute); ended != MinimumRetryAfter {
		t.Fatalf("a window that ended an hour ago reported a wait of %s, want %s", ended, MinimumRetryAfter)
	}
	live := remaining(time.Now(), time.Minute)
	if live < MinimumRetryAfter || live > time.Minute {
		t.Fatalf("a window with a minute left reported a wait of %s", live)
	}
}
