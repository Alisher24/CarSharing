package billing

import "time"

// Mode names one of the two ways a ride spends time. A charge is made of one line per mode, and the
// contract publishes the lines in this order, driving first and paused second.
type Mode string

const (
	Driving Mode = "driving"
	Paused  Mode = "paused"
)

// Line is what one mode of a ride costs: how long the ride spent in it, the minutes begun in that
// mode, the rate those minutes were priced at and their product. The four stand together because an
// invoice states all four, and a line whose minutes were priced at another rate than the one it
// publishes would be an invoice nobody could check.
type Line struct {
	Mode        Mode
	Duration    time.Duration
	Minutes     int64
	Rate        RateTyiynPerStartedMinute
	AmountTyiyn AmountTyiyn
}

// Lines is the two lines of a charge, in the order the contract publishes them: driving first and
// paused second. Both modes are always present, including one the ride never entered, because the
// contract states exactly two lines and says so.
func (c Charge) Lines() (driving Line, paused Line) {
	return Line{
		Mode:        Driving,
		Duration:    c.DrivingDuration,
		Minutes:     c.DrivingMinutes,
		Rate:        c.Rates.Driving,
		AmountTyiyn: c.DrivingAmount,
	}, Line{
		Mode:        Paused,
		Duration:    c.PausedDuration,
		Minutes:     c.PausedMinutes,
		Rate:        c.Rates.Paused,
		AmountTyiyn: c.PausedAmount,
	}
}

// Microseconds is a duration as the contract publishes it: whole microseconds rather than a
// time.Duration, which is counted in nanoseconds and cannot hold every count the contract admits.
func (l Line) Microseconds() Microseconds { return Microseconds(l.Duration.Microseconds()) }
