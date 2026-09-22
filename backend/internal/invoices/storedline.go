package invoices

import "github.com/Alisher24/CarSharing/backend/internal/billing"

type storedLine struct {
	duration int64
	minutes  int64
	rate     int64
}

func (line storedLine) decoded(mode billing.Mode) Line {
	return Line{
		Mode:        mode,
		Duration:    billing.Microseconds(line.duration),
		Minutes:     line.minutes,
		Rate:        billing.RateTyiynPerStartedMinute(line.rate),
		AmountTyiyn: billing.AmountTyiyn(line.minutes * line.rate),
	}
}
