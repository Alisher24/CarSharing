package simulation

import "github.com/Alisher24/CarSharing/backend/internal/fleet"

// Parameters are the capacities a ride is simulated with: how much of each source the vehicle holds
// when it is full. They are stored with the ride that began rather than read from the demonstrated
// fleet on every tick, so a renewed capacity or a different fleet applies from the next ride and never
// recomputes the past of one already under way.
//
// A rate is derived from the capacity rather than stored beside it: the two would otherwise be a second
// declaration of the same fact, and a rate that disagreed with the capacity it came from would spend a
// source at a speed its own reserve does not explain.
type Parameters struct {
	Capacities map[fleet.SourceKind]fleet.Amount
}

// ParametersOf snapshots what a fleet holds when each source is full.
func ParametersOf(capacities map[fleet.SourceKind]fleet.Amount) Parameters {
	snapshot := make(map[fleet.SourceKind]fleet.Amount, len(capacities))
	for kind, capacity := range capacities {
		snapshot[kind] = capacity
	}
	return Parameters{Capacities: snapshot}
}

// RateOf is how fast a source of this kind is spent. A kind the parameters do not name cannot be spent
// at all, which is what keeps a ride from quietly moving on a source nobody declared.
func (p Parameters) RateOf(kind fleet.SourceKind) Rate { return RateOf(p.Capacities[kind]) }
