package demo

import "github.com/Alisher24/CarSharing/backend/internal/tariffs"

// The demonstration prices, in whole tyiyn per started minute of each mode. The interface shows
// them as 12,34 and 3,21 som; nothing rounds them on the way there.
const (
	drivingRateTyiynPerStartedMinute = 1234
	pausedRateTyiynPerStartedMinute  = 321

	demoCurrency         = "KGS"
	demoBillingPolicy    = "per_mode_started_minute_v1"
	tariffInitialVersion = 1
)

// Tariff is the price list the demonstration installs.
func Tariff() tariffs.Tariff {
	return tariffs.Tariff{
		ID:                               resourceID(tariffFamily, 1),
		Currency:                         demoCurrency,
		BillingPolicy:                    demoBillingPolicy,
		DrivingRateTyiynPerStartedMinute: drivingRateTyiynPerStartedMinute,
		PausedRateTyiynPerStartedMinute:  pausedRateTyiynPerStartedMinute,
		Version:                          tariffInitialVersion,
	}
}
