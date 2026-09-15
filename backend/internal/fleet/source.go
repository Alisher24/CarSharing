package fleet

// SourceKind is one kind of energy a vehicle can carry.
type SourceKind string

const (
	SourceBattery  SourceKind = "battery"
	SourceGasoline SourceKind = "gasoline"
	SourceDiesel   SourceKind = "diesel"
	SourceLPG      SourceKind = "lpg"
	SourceCNG      SourceKind = "cng"
)

// WGS84SRID is the spatial reference every stored coordinate uses, so a position written by one
// module and read by another describe the same place.
const WGS84SRID = 4326

// SourceNames renders sources the way the columns and the wire spell them: one word per kind, in the
// order given. A ride that ran out of nothing states that by holding nothing rather than by naming a
// source it never carried.
func SourceNames(kinds []SourceKind) []string {
	if len(kinds) == 0 {
		return nil
	}
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, string(kind))
	}
	return names
}

// SourceKinds reads stored source names back in the vocabulary the catalog publishes them in.
func SourceKinds(names []string) []SourceKind {
	if len(names) == 0 {
		return nil
	}
	kinds := make([]SourceKind, 0, len(names))
	for _, name := range names {
		kinds = append(kinds, SourceKind(name))
	}
	return kinds
}

// Unit is how a source's inventory is measured. Which unit a kind uses is a property of the kind
// rather than of the vehicle, so it is declared once here and never stored beside an inventory.
type Unit string

const (
	WattHours   Unit = "wh"
	Millilitres Unit = "ml"
	Grams       Unit = "g"
)

var sourceUnits = map[SourceKind]Unit{
	SourceBattery:  WattHours,
	SourceGasoline: Millilitres,
	SourceDiesel:   Millilitres,
	SourceLPG:      Millilitres,
	SourceCNG:      Grams,
}

// UnitOf returns the unit a source kind is measured in, and reports whether the kind is known.
func UnitOf(kind SourceKind) (Unit, bool) {
	unit, known := sourceUnits[kind]
	return unit, known
}
