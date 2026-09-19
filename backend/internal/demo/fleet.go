package demo

import (
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
)

// membersPerPowertrain is how many vehicles each powertrain contributes: two a person can book,
// one reserved, one in a trip and one that has run its reserves down. Every public state is
// therefore present in every group.
const membersPerPowertrain = 5

// The round quantities a demonstration capacity is stated in.
const (
	wattHour = fleet.AmountScale
	litre    = 1000 * fleet.AmountScale
)

// member is one prepared vehicle of a group: what it holds, what holds it and which failure it
// stands in for. The two negative fields are written only where they apply, so an ordinary vehicle
// reads as ordinary.
type member struct {
	// heldBy is the stage of the rental prepared for this vehicle, or stage.NotHeld when the
	// vehicle is left for a person to book.
	heldBy stage.Stage

	// unlinked is a vehicle the platform has no link to, which reports offline however recent its
	// last reading was.
	unlinked bool

	// silent is a linked vehicle the demonstration source stops confirming, so that its position
	// genuinely ages past the freshness limit rather than being labelled stale.
	silent bool

	// charge is how full each source is, in basis points of its capacity.
	charge map[fleet.SourceKind]int
}

// group is the five demonstration vehicles of one powertrain, which circuit each of them drives and
// what each of them holds.
type group struct {
	powertrain  fleet.PowertrainType
	modelPrefix string
	capacities  map[fleet.SourceKind]fleet.Amount

	// routes and members are read by the same index: the vehicle on routes[n] is the one members[n]
	// declares, and a group that gained a member names which circuit it drives in the same declaration
	// rather than in a list kept beside it.
	routes  [membersPerPowertrain]simulation.RouteID
	members [membersPerPowertrain]member
}

// groups is the whole demonstration fleet. Every group carries the reserves the rules turn on: one
// vehicle at or just below the start threshold, and, where the powertrain has more than one
// source, reserves spread across them so that they must not be added together.
var groups = []group{
	{
		powertrain:  fleet.PowertrainElectric,
		modelPrefix: "Демо Электро",
		capacities:  map[fleet.SourceKind]fleet.Amount{fleet.SourceBattery: 60_000 * wattHour},
		routes: [membersPerPowertrain]simulation.RouteID{
			"electric-1",
			"electric-2",
			"electric-3",
			"electric-4",
			"electric-5",
		},
		members: [membersPerPowertrain]member{
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 8200}},
			// Exactly the start threshold, which is enough.
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 2000}},
			{heldBy: stage.Reserved, charge: map[fleet.SourceKind]int{fleet.SourceBattery: 7400}},
			{heldBy: stage.Active, charge: map[fleet.SourceKind]int{fleet.SourceBattery: 5600}},
			// One ten-thousandth below the threshold, which is not.
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 1999}},
		},
	},
	{
		powertrain:  fleet.PowertrainGasoline,
		modelPrefix: "Демо Бензин",
		capacities:  map[fleet.SourceKind]fleet.Amount{fleet.SourceGasoline: 50 * litre},
		routes: [membersPerPowertrain]simulation.RouteID{
			"gasoline-1",
			"gasoline-2",
			"gasoline-3",
			"gasoline-4",
			"gasoline-5",
		},
		members: [membersPerPowertrain]member{
			{charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 9100}},
			{charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 4300}},
			{heldBy: stage.Reserved, charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 6600}},
			{heldBy: stage.Paused, charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 3800}},
			{silent: true, charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 1500}},
		},
	},
	{
		powertrain:  fleet.PowertrainDiesel,
		modelPrefix: "Демо Дизель",
		capacities:  map[fleet.SourceKind]fleet.Amount{fleet.SourceDiesel: 55 * litre},
		routes: [membersPerPowertrain]simulation.RouteID{
			// The one vehicle whose circuit leaves the demonstration area, so that an ending beyond
			// the boundary can be shown on it.
			simulation.ScenarioRouteID,
			"diesel-2",
			"diesel-3",
			"diesel-4",
			"diesel-5",
		},
		members: [membersPerPowertrain]member{
			{charge: map[fleet.SourceKind]int{fleet.SourceDiesel: 8800}},
			{charge: map[fleet.SourceKind]int{fleet.SourceDiesel: 3100}},
			{heldBy: stage.Reserved, charge: map[fleet.SourceKind]int{fleet.SourceDiesel: 7000}},
			{heldBy: stage.Active, charge: map[fleet.SourceKind]int{fleet.SourceDiesel: 4500}},
			{unlinked: true, charge: map[fleet.SourceKind]int{fleet.SourceDiesel: 1200}},
		},
	},
	{
		powertrain:  fleet.PowertrainHybrid,
		modelPrefix: "Демо Гибрид",
		capacities: map[fleet.SourceKind]fleet.Amount{
			fleet.SourceBattery:  12_000 * wattHour,
			fleet.SourceGasoline: 45 * litre,
		},
		routes: [membersPerPowertrain]simulation.RouteID{
			"hybrid-1",
			"hybrid-2",
			"hybrid-3",
			"hybrid-4",
			"hybrid-5",
		},
		members: [membersPerPowertrain]member{
			// An empty battery beside a sufficient tank: the tank alone makes the vehicle fit.
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 0, fleet.SourceGasoline: 6000}},
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 5000, fleet.SourceGasoline: 1000}},
			{
				heldBy: stage.Reserved,
				charge: map[fleet.SourceKind]int{fleet.SourceBattery: 3000, fleet.SourceGasoline: 4000},
			},
			{
				heldBy: stage.Paused,
				charge: map[fleet.SourceKind]int{fleet.SourceBattery: 1500, fleet.SourceGasoline: 3300},
			},
			// Both sources below the threshold: nineteen per cent twice is not thirty-eight.
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 1900, fleet.SourceGasoline: 1900}},
		},
	},
	{
		powertrain:  fleet.PowertrainGas,
		modelPrefix: "Демо Газ",
		capacities: map[fleet.SourceKind]fleet.Amount{
			fleet.SourceGasoline: 50 * litre,
			fleet.SourceLPG:      60 * litre,
		},
		routes: [membersPerPowertrain]simulation.RouteID{
			"gas-1",
			"gas-2",
			"gas-3",
			"gas-4",
			"gas-5",
		},
		members: [membersPerPowertrain]member{
			{charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 7200, fleet.SourceLPG: 6400}},
			{charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 1000, fleet.SourceLPG: 5000}},
			{
				heldBy: stage.Reserved,
				charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 5500, fleet.SourceLPG: 3000},
			},
			{
				heldBy: stage.Active,
				charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 4100, fleet.SourceLPG: 2200},
			},
			{charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 1800, fleet.SourceLPG: 1700}},
		},
	},
}

// Vehicle is one vehicle of the demonstration fleet, resolved from its group and its member.
type Vehicle struct {
	ID             string
	Model          string
	PowertrainType fleet.PowertrainType
	Position       fleet.Position
	Connected      bool
	Reporting      bool
	Sources        []fleet.EnergySource

	// RouteID is the circuit the vehicle travels, which is what the model drives it along. A vehicle
	// without one is not moved by the model at all.
	RouteID simulation.RouteID

	// HeldBy is the stage of the rental prepared for this vehicle, or stage.NotHeld when none is.
	HeldBy stage.Stage

	// ScenarioAccount is the service account the prepared rental belongs to, empty for a vehicle
	// no rental is prepared for. A prepared rental never belongs to a person's own account.
	ScenarioAccount string

	// PreparedRentalID names the rental prepared for this vehicle, empty when none is. Installing
	// the scenario and restoring it name the same row, so neither adds a second copy of it.
	PreparedRentalID string
}

// staleByMargin is how far past the freshness limit an unconfirmed reading is written, so that a
// vehicle the source skips is already stale the moment the scenario is installed or put back.
const staleByMargin = time.Second

// confirmedAgo is how long ago this vehicle last reported. A reporting vehicle has just been
// confirmed; one the source skips has not been confirmed within the freshness limit.
func (v Vehicle) confirmedAgo() time.Duration {
	if v.Reporting {
		return 0
	}
	return fleet.MaxTelemetryAge + staleByMargin
}

// fitToStart asks the domain rule rather than restating it, so a declared vehicle is judged by
// exactly what the catalog will judge it by.
func (v Vehicle) fitToStart() bool {
	return fleet.Vehicle{PowertrainType: v.PowertrainType, Sources: v.Sources}.FitToStart()
}

// Fleet resolves the whole declared fleet, in the order the demonstration numbers it.
func Fleet() []Vehicle {
	vehicles := make([]Vehicle, 0, len(groups)*membersPerPowertrain)
	for groupIndex, current := range groups {
		for positionInGroup, declared := range current.members {
			vehicles = append(vehicles, current.vehicle(declared, groupIndex, positionInGroup, len(vehicles)+1))
		}
	}
	return vehicles
}

func (g group) vehicle(declared member, groupIndex, positionInGroup, number int) Vehicle {
	route := g.route(positionInGroup)
	vehicle := Vehicle{
		ID:             resourceID(vehicleFamily, number),
		Model:          fmt.Sprintf("%s %d", g.modelPrefix, positionInGroup+1),
		PowertrainType: g.powertrain,
		Position:       spotAlong(route, groupIndex, positionInGroup),
		RouteID:        route.ID,
		Connected:      !declared.unlinked,
		Reporting:      !declared.unlinked && !declared.silent,
		Sources:        g.sources(declared),
		HeldBy:         declared.heldBy,
	}
	if vehicle.HeldBy != stage.NotHeld {
		vehicle.ScenarioAccount = scenarioAccountAddress(number)
		vehicle.PreparedRentalID = resourceID(rentalFamily, number)
	}
	return vehicle
}

// spotAlong is where one member stands: its own step around its own circuit, moved on by the place its
// group has in the declaration. A step is how far into the group the member is, and the group's place
// is added because the circuits are nested rather than disjoint — they share the corner the innermost
// of them begins at, so five vehicles each starting at the beginning of their own ring would still
// stand on one point, and a reserved or paused vehicle never leaves the place it was installed at: the
// markers would be drawn over each other and a person could not press the ones underneath.
func spotAlong(route simulation.Route, groupIndex, positionInGroup int) fleet.Position {
	steps := groupIndex + positionInGroup
	spots := len(groups) * membersPerPowertrain
	lap := route.Length()
	return route.PositionAt(simulation.Path(int64(lap) * int64(steps) / int64(spots)))
}

// route is the circuit the member this group places at an index drives. A group naming a circuit this
// build does not declare is a defect of the declaration rather than a condition to carry on from, and
// finding it here is what keeps it from reaching a client as a vehicle standing nowhere.
func (g group) route(positionInGroup int) simulation.Route {
	route, declared := simulation.RouteOf(g.routes[positionInGroup])
	if !declared {
		panic(fmt.Sprintf("%s drives the undeclared circuit %q", g.modelPrefix, g.routes[positionInGroup]))
	}
	return route
}

// sources resolves the member's reserves against its powertrain's capacities, in the order the
// profile lists them so that two readings of the declaration agree.
func (g group) sources(declared member) []fleet.EnergySource {
	profile, known := fleet.ProfileOf(g.powertrain)
	if !known {
		return nil
	}
	sources := make([]fleet.EnergySource, 0, len(profile.Sources))
	for _, kind := range profile.Sources {
		capacity := g.capacities[kind]
		sources = append(sources, fleet.EnergySource{
			Kind:      kind,
			Capacity:  capacity,
			Remaining: fleet.Amount(int64(capacity) * int64(declared.charge[kind]) / fleet.WholeInBasisPoints),
		})
	}
	return sources
}
