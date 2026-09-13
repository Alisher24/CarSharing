package demo

import (
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
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
	// heldBy is the stage of the rental prepared for this vehicle, or rentals.NotHeld when the
	// vehicle is left for a person to book.
	heldBy rentals.Stage

	// unlinked is a vehicle the platform has no link to, which reports offline however recent its
	// last reading was.
	unlinked bool

	// silent is a linked vehicle the demonstration source stops confirming, so that its position
	// genuinely ages past the freshness limit rather than being labelled stale.
	silent bool

	// charge is how full each source is, in basis points of its capacity.
	charge map[fleet.SourceKind]int
}

// group is the five demonstration vehicles of one powertrain, and where each of them stands.
type group struct {
	powertrain  fleet.PowertrainType
	modelPrefix string
	capacities  map[fleet.SourceKind]fleet.Amount

	// positions and members are read by the same index: the vehicle that stands at positions[n] is
	// the one members[n] declares, and a group that gained a member states where it stands in the
	// same declaration rather than in a list of positions kept beside it.
	positions [membersPerPowertrain]fleet.Position
	members   [membersPerPowertrain]member
}

// groups is the whole demonstration fleet. Every group carries the reserves the rules turn on: one
// vehicle at or just below the start threshold, and, where the powertrain has more than one
// source, reserves spread across them so that they must not be added together.
var groups = []group{
	{
		powertrain:  fleet.PowertrainElectric,
		modelPrefix: "Демо Электро",
		capacities:  map[fleet.SourceKind]fleet.Amount{fleet.SourceBattery: 60_000 * wattHour},
		positions: [membersPerPowertrain]fleet.Position{
			{Longitude: 74.5720, Latitude: 42.8590},
			{Longitude: 74.5865, Latitude: 42.8742},
			{Longitude: 74.5990, Latitude: 42.8663},
			{Longitude: 74.6120, Latitude: 42.8815},
			{Longitude: 74.6285, Latitude: 42.8574},
		},
		members: [membersPerPowertrain]member{
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 8200}},
			// Exactly the start threshold, which is enough.
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 2000}},
			{heldBy: rentals.Reserved, charge: map[fleet.SourceKind]int{fleet.SourceBattery: 7400}},
			{heldBy: rentals.Active, charge: map[fleet.SourceKind]int{fleet.SourceBattery: 5600}},
			// One ten-thousandth below the threshold, which is not.
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 1999}},
		},
	},
	{
		powertrain:  fleet.PowertrainGasoline,
		modelPrefix: "Демо Бензин",
		capacities:  map[fleet.SourceKind]fleet.Amount{fleet.SourceGasoline: 50 * litre},
		positions: [membersPerPowertrain]fleet.Position{
			{Longitude: 74.5638, Latitude: 42.8871},
			{Longitude: 74.5793, Latitude: 42.8486},
			{Longitude: 74.6046, Latitude: 42.8928},
			{Longitude: 74.6209, Latitude: 42.8701},
			{Longitude: 74.6371, Latitude: 42.8836},
		},
		members: [membersPerPowertrain]member{
			{charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 9100}},
			{charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 4300}},
			{heldBy: rentals.Reserved, charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 6600}},
			{heldBy: rentals.Paused, charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 3800}},
			{silent: true, charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 1500}},
		},
	},
	{
		powertrain:  fleet.PowertrainDiesel,
		modelPrefix: "Демо Дизель",
		capacities:  map[fleet.SourceKind]fleet.Amount{fleet.SourceDiesel: 55 * litre},
		positions: [membersPerPowertrain]fleet.Position{
			{Longitude: 74.5561, Latitude: 42.8628},
			{Longitude: 74.5904, Latitude: 42.8955},
			{Longitude: 74.6158, Latitude: 42.8443},
			{Longitude: 74.6432, Latitude: 42.8759},
			{Longitude: 74.5682, Latitude: 42.8794},
		},
		members: [membersPerPowertrain]member{
			{charge: map[fleet.SourceKind]int{fleet.SourceDiesel: 8800}},
			{charge: map[fleet.SourceKind]int{fleet.SourceDiesel: 3100}},
			{heldBy: rentals.Reserved, charge: map[fleet.SourceKind]int{fleet.SourceDiesel: 7000}},
			{heldBy: rentals.Active, charge: map[fleet.SourceKind]int{fleet.SourceDiesel: 4500}},
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
		positions: [membersPerPowertrain]fleet.Position{
			{Longitude: 74.5837, Latitude: 42.8617},
			{Longitude: 74.6091, Latitude: 42.8880},
			{Longitude: 74.6246, Latitude: 42.8521},
			{Longitude: 74.6398, Latitude: 42.8646},
			{Longitude: 74.5599, Latitude: 42.8912},
		},
		members: [membersPerPowertrain]member{
			// An empty battery beside a sufficient tank: the tank alone makes the vehicle fit.
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 0, fleet.SourceGasoline: 6000}},
			{charge: map[fleet.SourceKind]int{fleet.SourceBattery: 5000, fleet.SourceGasoline: 1000}},
			{
				heldBy: rentals.Reserved,
				charge: map[fleet.SourceKind]int{fleet.SourceBattery: 3000, fleet.SourceGasoline: 4000},
			},
			{
				heldBy: rentals.Paused,
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
		positions: [membersPerPowertrain]fleet.Position{
			{Longitude: 74.5751, Latitude: 42.8703},
			{Longitude: 74.6014, Latitude: 42.8558},
			{Longitude: 74.6177, Latitude: 42.8967},
			{Longitude: 74.6320, Latitude: 42.8688},
			{Longitude: 74.5926, Latitude: 42.8461},
		},
		members: [membersPerPowertrain]member{
			{charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 7200, fleet.SourceLPG: 6400}},
			{charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 1000, fleet.SourceLPG: 5000}},
			{
				heldBy: rentals.Reserved,
				charge: map[fleet.SourceKind]int{fleet.SourceGasoline: 5500, fleet.SourceLPG: 3000},
			},
			{
				heldBy: rentals.Active,
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

	// HeldBy is the stage of the rental prepared for this vehicle, or rentals.NotHeld when none is.
	HeldBy rentals.Stage

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

// Restored reports whether the scenario command puts this vehicle back. A vehicle that is free and
// fit to drive is one a person books by hand, and the command never touches it.
func (v Vehicle) Restored() bool {
	return v.HeldBy != rentals.NotHeld || !v.fitToStart()
}

// fitToStart asks the domain rule rather than restating it, so a declared vehicle is judged by
// exactly what the catalog will judge it by.
func (v Vehicle) fitToStart() bool {
	return fleet.Vehicle{PowertrainType: v.PowertrainType, Sources: v.Sources}.FitToStart()
}

// Fleet resolves the whole declared fleet, in the order the demonstration numbers it.
func Fleet() []Vehicle {
	vehicles := make([]Vehicle, 0, len(groups)*membersPerPowertrain)
	for _, current := range groups {
		for positionInGroup, declared := range current.members {
			vehicles = append(vehicles, current.vehicle(declared, positionInGroup, len(vehicles)+1))
		}
	}
	return vehicles
}

func (g group) vehicle(declared member, positionInGroup, number int) Vehicle {
	vehicle := Vehicle{
		ID:             resourceID(vehicleFamily, number),
		Model:          fmt.Sprintf("%s %d", g.modelPrefix, positionInGroup+1),
		PowertrainType: g.powertrain,
		Position:       g.positions[positionInGroup],
		Connected:      !declared.unlinked,
		Reporting:      !declared.unlinked && !declared.silent,
		Sources:        g.sources(declared),
		HeldBy:         declared.heldBy,
	}
	if vehicle.HeldBy != rentals.NotHeld {
		vehicle.ScenarioAccount = scenarioAccountAddress(number)
		vehicle.PreparedRentalID = resourceID(rentalFamily, number)
	}
	return vehicle
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
