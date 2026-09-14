package demo

import (
	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
)

// The demonstration prices, in whole tyiyn per started minute of each mode. The interface shows
// them as 12,34 and 3,21 som; nothing rounds them on the way there.
const (
	drivingRateTyiynPerStartedMinute = 1234
	pausedRateTyiynPerStartedMinute  = 321

	demoCurrency         = "KGS"
	tariffInitialVersion = 1
)

// Tariff is the price list the demonstration installs. It charges by the policy the billing module
// applies, named here rather than spelled again, so a price list cannot claim one rule while the
// arithmetic follows another.
func Tariff() tariffs.Tariff {
	return tariffs.Tariff{
		ID:                               resourceID(tariffFamily, 1),
		Currency:                         demoCurrency,
		BillingPolicy:                    billing.PolicyPerModeStartedMinuteV1,
		DrivingRateTyiynPerStartedMinute: drivingRateTyiynPerStartedMinute,
		PausedRateTyiynPerStartedMinute:  pausedRateTyiynPerStartedMinute,
		Version:                          tariffInitialVersion,
	}
}
