package fleet

const (
	// startThresholdBasisPoints is the reserve one source must still hold for a rental to start.
	// Reserves of different sources are never added together: the threshold is met by one source
	// or it is not met at all.
	startThresholdBasisPoints = 2000

	// WholeInBasisPoints is a full tank or a full battery expressed in the unit the contract
	// publishes a remainder in.
	WholeInBasisPoints = 10_000
)

// EnergySource is one inventory a vehicle carries.
type EnergySource struct {
	Kind      SourceKind
	Remaining Amount
	Capacity  Amount
}

// RemainingBasisPoints is how much of the capacity is left, rounded down, so a reserve just short
// of a threshold never reads as having reached it.
func (s EnergySource) RemainingBasisPoints() int {
	if s.Capacity <= 0 {
		return 0
	}
	return int(int64(s.Remaining) * WholeInBasisPoints / int64(s.Capacity))
}

// MeetsStartThreshold reports whether this source alone holds enough to begin a rental. Exactly the
// threshold is enough.
func (s EnergySource) MeetsStartThreshold() bool {
	return s.RemainingBasisPoints() >= startThresholdBasisPoints
}

// CanContinue reports whether a rental already under way can go on using this source. Any positive
// reserve will do: the start threshold is not applied a second time.
func (s EnergySource) CanContinue() bool {
	return s.Remaining > 0
}
