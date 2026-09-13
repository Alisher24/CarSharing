package timestamp

import (
	"testing"
	"time"
)

// The contract declares six fractional digits exactly, so a value with more precision is truncated
// rather than rounded or written out in full.
func TestFormatStatesMicrosecondsOnly(t *testing.T) {
	for _, instant := range []struct {
		name  string
		given time.Time
		want  string
	}{
		{"whole second", time.Date(2026, time.September, 12, 7, 15, 30, 0, time.UTC), "2026-09-12T07:15:30.000000Z"},
		{"microseconds", time.Date(2026, time.September, 12, 7, 15, 30, 123456000, time.UTC), "2026-09-12T07:15:30.123456Z"},
		{"nanoseconds are dropped", time.Date(2026, time.September, 12, 7, 15, 30, 123456789, time.UTC), "2026-09-12T07:15:30.123456Z"},
	} {
		t.Run(instant.name, func(t *testing.T) {
			if got := Format(instant.given); got != instant.want {
				t.Fatalf("Format = %s, want %s", got, instant.want)
			}
		})
	}
}

// A stored time arrives in whatever zone the database connection states, and the wire format is
// always UTC, so the same instant is spelled the same way wherever it was read.
func TestFormatStatesTheInstantInUTC(t *testing.T) {
	bishkek := time.FixedZone("Asia/Bishkek", 6*60*60)
	instant := time.Date(2026, time.September, 12, 13, 15, 30, 0, bishkek)
	if got, want := Format(instant), "2026-09-12T07:15:30.000000Z"; got != want {
		t.Fatalf("Format = %s, want %s", got, want)
	}
}
