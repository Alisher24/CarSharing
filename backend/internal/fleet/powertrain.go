package fleet

// PowertrainType is how a vehicle is driven. The set is closed: a vehicle outside it has no
// profile, so nothing can decide whether it may start.
type PowertrainType string

const (
	PowertrainElectric PowertrainType = "electric"
	PowertrainGasoline PowertrainType = "gasoline"
	PowertrainDiesel   PowertrainType = "diesel"
	PowertrainHybrid   PowertrainType = "hybrid"
	PowertrainGas      PowertrainType = "gas"
)

// Profile is the powertrain's energy layout: the source kinds a vehicle of this powertrain carries,
// in the order it uses them. The first kind that still holds a reserve is the one the vehicle moves
// on, so this list is both what the vehicle carries and which of them it burns first. Every kind a
// profile lists can move the vehicle on its own, which is what makes its reserve count towards the
// start threshold on its own.
type Profile struct {
	PowertrainType PowertrainType
	Sources        []SourceKind
}

// profiles is the one declaration of the powertrains and what each of them carries, in the order
// the catalog and its demonstration data present them. Adding a powertrain is an entry here; both
// the lookup below and the ordered list are derived from it, so nothing else enumerates them.
var profiles = []Profile{
	{PowertrainType: PowertrainElectric, Sources: []SourceKind{SourceBattery}},
	{PowertrainType: PowertrainGasoline, Sources: []SourceKind{SourceGasoline}},
	{PowertrainType: PowertrainDiesel, Sources: []SourceKind{SourceDiesel}},
	{PowertrainType: PowertrainHybrid, Sources: []SourceKind{SourceBattery, SourceGasoline}},
	{PowertrainType: PowertrainGas, Sources: []SourceKind{SourceLPG, SourceGasoline}},
}

var profileByPowertrain = indexProfiles(profiles)

func indexProfiles(declared []Profile) map[PowertrainType]Profile {
	indexed := make(map[PowertrainType]Profile, len(declared))
	for _, profile := range declared {
		indexed[profile.PowertrainType] = profile
	}
	return indexed
}

// ProfileOf returns the layout of a powertrain, and reports whether that powertrain is known.
func ProfileOf(powertrain PowertrainType) (Profile, bool) {
	profile, known := profileByPowertrain[powertrain]
	return profile, known
}

// PowertrainTypes lists every powertrain the fleet can carry, in declaration order.
func PowertrainTypes() []PowertrainType {
	powertrains := make([]PowertrainType, 0, len(profiles))
	for _, profile := range profiles {
		powertrains = append(powertrains, profile.PowertrainType)
	}
	return powertrains
}

// DrivesAlone reports whether a source of this kind can move a vehicle of this powertrain by
// itself. A kind the profile does not carry never can, so its reserve is not a reason to start.
func (p Profile) DrivesAlone(kind SourceKind) bool {
	for _, carried := range p.Sources {
		if carried == kind {
			return true
		}
	}
	return false
}
