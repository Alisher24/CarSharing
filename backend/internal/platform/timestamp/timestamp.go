// Package timestamp renders the instant format the API contract declares: UTC, RFC3339 and exactly
// six fractional digits. Every server time and stored time a response states is written in it, so
// the format is declared once here rather than by each feature that has an instant to publish.
package timestamp

import "time"

// Layout is the one spelling of an instant. It fixes the fractional part at microseconds, so a
// stored time and a server time never appear in two shapes inside one response.
const Layout = "2006-01-02T15:04:05.000000Z"

// Format renders an instant the way the contract's Timestamp declares it. Sub-microsecond precision
// is dropped rather than rounded, because the database stores microseconds and a rounded value would
// name a moment that was never read.
func Format(instant time.Time) string {
	return instant.UTC().Truncate(time.Microsecond).Format(Layout)
}
