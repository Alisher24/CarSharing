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
