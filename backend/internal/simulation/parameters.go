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

// InProfileOrder arranges inventories in the order a powertrain uses them. The order is the whole of
// the priority rule: the first source that still holds something is the one the vehicle moves on, so
// a list in any other order would burn the reserve the profile says to keep. An inventory the profile
// does not name is left out, because nothing would ever move on it.
func InProfileOrder(sources []Source, profile []fleet.SourceKind) []Source {
	ordered := make([]Source, 0, len(profile))
	for _, kind := range profile {
		for _, source := range sources {
			if source.Kind == kind {
				ordered = append(ordered, source)
				break
			}
		}
	}
	return ordered
}
