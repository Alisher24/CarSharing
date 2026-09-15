package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
)

// Reconciled is what bringing the model of one vehicle to a moment decided.
type Reconciled struct {
	// Ended is the ride the model ran out of, if it did. Its ending, its invoice and its report are
	// written before this is returned.
	Ended *Rental

	// Moved reports that the vehicle travelled or spent something, which is what a tick publishes as
	// a changed vehicle.
	Moved bool
}

// ReconciledVehicle is what the model decided about one vehicle of the fleet, named so that a tick can
// report what it changed.
type ReconciledVehicle struct {
	VehicleID string
	Reconciled
}

// reconcileVehicle brings the model of one named vehicle to a moment. It reads the vehicle and acts on
// the rental the caller hands it, so the mode the model is moved in is the one the caller judged under
// the locks it holds.
//
// A vehicle this installation does not publish a simulated reading for — one no route was placed on,
// or one that has never confirmed a position — is left exactly as it is rather than guessed at.
func (s *Service) reconcileVehicle(
	ctx context.Context, moment time.Time, vehicleID string, held *Rental,
) (Reconciled, error) {
	vehicle, err := s.vehicles.SimulatedVehicle(ctx, vehicleID)
	if errors.Is(err, fleet.ErrVehicleNotFound) {
		return Reconciled{}, nil
	}
	if err != nil {
		return Reconciled{}, err
	}
	return s.reconcileRead(ctx, moment, vehicle, held)
}

// reconcileFleet brings every vehicle the model travels to a moment, in the order the fleet is read.
// The rentals are read once for the whole fleet, because the mode of every vehicle comes from the one
// rental that holds it and the transaction already holds those rows.
func (s *Service) reconcileFleet(ctx context.Context, moment time.Time) ([]ReconciledVehicle, error) {
	vehicles, err := s.vehicles.Simulated(ctx)
	if err != nil {
		return nil, err
	}
	held, err := liveRentals(ctx, s.pool)
	if err != nil {
		return nil, err
	}
	holders := make(map[string]*Rental, len(held))
	for index := range held {
		holders[held[index].VehicleID] = &held[index]
	}

	outcomes := make([]ReconciledVehicle, 0, len(vehicles))
	for _, vehicle := range vehicles {
		outcome, err := s.reconcileRead(ctx, moment, vehicle, holders[vehicle.ID])
		if err != nil {
			return nil, err
		}
		outcomes = append(outcomes, ReconciledVehicle{VehicleID: vehicle.ID, Reconciled: outcome})
	}
	return outcomes, nil
}

// reconcileRead is the model's whole meeting with time for one vehicle: it restores what was stored
// for it — or takes its confirmed position and reserves as the beginning of its journey — plays the
// window since then in the mode the vehicle is in, saves what it reached, and ends the ride if the
// sources ran out.
func (s *Service) reconcileRead(
	ctx context.Context, moment time.Time, vehicle fleet.SimulatedVehicle, held *Rental,
) (Reconciled, error) {
	state, stored, err := s.restore(ctx, moment, vehicle)
	if err != nil {
		return Reconciled{}, err
	}
	if state.ProcessedAt.After(moment) {
		// The model has already accounted for time this command has not reached. Playing it again
		// would spend a reserve it never had, so the two sides are told apart rather than reconciled.
		return Reconciled{}, simulation.ErrTimeReversed
	}

	mode := modeOf(held)
	motion, advanced, err := advance(state, mode, moment)
	if err != nil {
		return Reconciled{}, err
	}
	// The state is saved whenever a window was played, whatever the vehicle did in it. A free or
	// reserved vehicle spends nothing, but the moment it accounts for has still moved: leaving it
	// behind would make the next reconcile play that window again, in the mode the vehicle is in by
	// then — the whole of a free afternoon charged as driving the moment a ride starts.
	spent := spends(mode)
	if spent || !stored || state.ProcessedAt.Before(moment) {
		if err = s.models.Save(ctx, vehicle.ID, advanced); err != nil {
			return Reconciled{}, err
		}
	}
	ranOut, depleted := motion.DepletedBy()
	if !depleted {
		return Reconciled{Moved: spent}, nil
	}

	ending := Depleted(wholeMicrosecondFirstAfter(ranOut), exhaustedBy(motion.Holding, advanced.Sources))
	if held == nil || !held.Riding() {
		// Nothing was riding the vehicle, so there is no ride to end: the model has run out standing
		// still, which only a reserve that was already empty can do.
		return Reconciled{Moved: spent}, nil
	}
	if _, err = s.endRide(ctx, moment, *held, ending); err != nil {
		return Reconciled{}, err
	}
	return Reconciled{Ended: held, Moved: true}, nil
}

// restore reads the model of one vehicle: the state stored for it, or the state its confirmed position
// and reserves begin at. A vehicle the model has never moved starts where it stands, with the moment
// of this reading as the first moment it accounts for: the time before the model existed is not
// invented, because nothing recorded what the vehicle did in it.
func (s *Service) restore(
	ctx context.Context, moment time.Time, vehicle fleet.SimulatedVehicle,
) (simulation.State, bool, error) {
	stored, err := s.models.States(ctx, []string{vehicle.ID})
	if err != nil {
		return simulation.State{}, false, err
	}
	if found, known := stored[vehicle.ID]; known {
		return found, true, nil
	}
	begun, err := begin(vehicle, moment)
	if err != nil {
		return simulation.State{}, false, err
	}
	return begun, false, nil
}

// begin is the state one vehicle's journey starts from: the route it was placed on, what it holds,
// and where along that route it stands. The route passes through the place the vehicle stands, so
// beginning at the point of the route nearest to it is beginning where it is.
func begin(vehicle fleet.SimulatedVehicle, moment time.Time) (simulation.State, error) {
	route, declared := simulation.RouteOf(simulation.RouteID(vehicle.RouteID))
	if !declared {
		return simulation.State{}, simulation.ErrStateUnusable
	}
	profile, known := fleet.ProfileOf(vehicle.PowertrainType)
	if !known {
		return simulation.State{}, simulation.ErrStateUnusable
	}
	carried := make([]simulation.Source, 0, len(vehicle.Sources))
	for _, source := range vehicle.Sources {
		carried = append(carried, simulation.SourceWith(source.Kind, source.Capacity, source.Remaining))
	}
	sources := simulation.InProfileOrder(carried, profile.Sources)
	if len(sources) == 0 {
		return simulation.State{}, simulation.ErrStateUnusable
	}

	path, _ := route.DistanceFrom(vehicle.Position)
	return simulation.State{
		RouteID:     route.ID,
		Parameters:  simulation.ParametersOf(capacities(sources)),
		Sources:     sources,
		Path:        path,
		Position:    route.PositionAt(path),
		ProcessedAt: moment,
	}, nil
}

// capacities is what each source holds when it is full, which is the whole of what the parameters of
// a ride state.
func capacities(sources []simulation.Source) map[fleet.SourceKind]fleet.Amount {
	full := make(map[fleet.SourceKind]fleet.Amount, len(sources))
	for _, source := range sources {
		full[source.Kind] = source.Capacity
	}
	return full
}

// advance plays one window of one vehicle's journey. A vehicle already at the moment asked about has
// nothing to play, which is what its holding at that moment answers with.
func advance(
	state simulation.State, mode simulation.Mode, moment time.Time,
) (simulation.Motion, simulation.State, error) {
	if state.ProcessedAt.Equal(moment) {
		return simulation.Motion{Position: state.Position, Holding: state.Holding()}, state, nil
	}
	spans := []simulation.Span{{Mode: mode, From: state.ProcessedAt, To: moment}}
	return state.Journey(spans, moment)
}

// modeOf is what the vehicle does between the moment its state describes and the moment a command
// acts on: a ride that is moving spends at the driving rate, one that is standing still at the paused
// rate, and a reserved or unheld vehicle neither moves nor spends.
func modeOf(held *Rental) simulation.Mode {
	if held == nil {
		return simulation.Free
	}
	switch held.Stage {
	case stage.Active:
		return simulation.Driving
	case stage.Paused:
		return simulation.Paused
	default:
		return simulation.Free
	}
}

// spends reports whether a vehicle in this mode uses any of its reserve at all, which is what a tick
// publishes as a change.
func spends(mode simulation.Mode) bool {
	return mode == simulation.Driving || mode == simulation.Paused
}

// exhaustedBy names the sources that were empty when a ride ran out, in the order the vehicle's
// profile uses them. It is read from what the model held at that moment rather than from what the
// vehicle holds later, so a refill after the ending cannot rewrite why the ride ended.
func exhaustedBy(holding []fleet.Amount, sources []simulation.Source) []fleet.SourceKind {
	empty := make([]fleet.SourceKind, 0, len(sources))
	for index, left := range holding {
		if index < len(sources) && left == 0 {
			empty = append(empty, sources[index].Kind)
		}
	}
	return empty
}

// wholeMicrosecondFirstAfter is the first whole microsecond at which the reserve is gone. The model
// states the moment a source ran out in nanoseconds and storage counts whole microseconds, so the
// moment an ending is stored at is carried forward rather than rounded: an invoice may not be dated
// before the reserve it priced had run out.
func wholeMicrosecondFirstAfter(moment time.Time) time.Time {
	whole := moment.Truncate(time.Microsecond)
	if whole.Equal(moment) {
		return moment
	}
	return whole.Add(time.Microsecond)
}
