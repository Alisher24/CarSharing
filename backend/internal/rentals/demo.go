package rentals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/demoaction"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
	"github.com/jackc/pgx/v5"
)

// DemoCommand is one demonstration command. Which fields it carries follows from its kind: a command
// that names a source states no position, and one that names a rental states no vehicle. The fields a
// kind does not use are read by nothing.
type DemoCommand struct {
	ActionID string
	Kind     demoaction.Kind
	Attempt  Attempt

	// VehicleID is the vehicle a change to its state names.
	VehicleID string

	// Online is whether a linked vehicle is being linked or unlinked, and Position where it is being
	// put.
	Online   bool
	Position fleet.Position

	// Source and Remaining are the source a refill names and what it is left holding.
	Source    fleet.SourceKind
	Remaining fleet.Amount

	// RentalID and Outcome are the ride whose next payment attempt is being decided, and what it
	// decides.
	RentalID string
	Outcome  invoices.DemoOutcome
}

// Validate reports why a command is not one this build can apply, or nil when it is. It guards the
// statements below from a kind whose values were never filled in, which is a defect of the caller
// rather than a refusal a client caused.
func (c DemoCommand) Validate() error {
	switch c.Kind {
	case demoaction.SetTelemetryState:
		return c.needsVehicle()
	case demoaction.SetPosition:
		return c.needsVehicle()
	case demoaction.SetEnergyRemaining:
		if err := c.needsVehicle(); err != nil {
			return err
		}
		if c.Source == "" {
			return errors.New("a refill must name the source it fills")
		}
		return nil
	case demoaction.MarkServiced:
		return c.needsVehicle()
	case demoaction.SetNextPaymentOutcome:
		if c.RentalID == "" {
			return errors.New("a payment outcome must name the ride it decides")
		}
		if !c.Outcome.Known() {
			return fmt.Errorf("a payment outcome cannot be %q", c.Outcome)
		}
		return nil
	default:
		return fmt.Errorf("the demonstration action %q is not one this build applies", c.Kind)
	}
}

func (c DemoCommand) needsVehicle() error {
	if c.VehicleID == "" {
		return fmt.Errorf("the demonstration action %q must name the vehicle it changes", c.Kind)
	}
	return nil
}

// ApplyDemo applies one demonstration command under the shared lock order, or answers why it cannot.
//
// The model of the vehicle is brought to the moment of the command before anything is changed by hand:
// a run-out that has already happened is over before the vehicle is refilled, which is what keeps a
// demonstration from erasing an ending that the service has not noticed yet.
func (s *Service) ApplyDemo(ctx context.Context, command DemoCommand) (Answered, error) {
	if err := command.Validate(); err != nil {
		return Answered{}, err
	}
	return s.answer(ctx, idempotency.ForInstallation(), command.Attempt,
		s.demoParticipants(command),
		func(ctx context.Context, tx pgx.Tx, moment time.Time) (Outcome, error) {
			return s.demoWithin(ctx, tx, moment, command)
		})
}

// demoParticipants is the rows a demonstration command touches: the vehicle and the rental that holds
// it, or the rental it names and the vehicle of that rental, together with their accounts.
func (s *Service) demoParticipants(command DemoCommand) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		planned := participants{}
		if command.VehicleID != "" {
			held, err := liveRentalOf(ctx, s.pool, vehicleLiveRentalSelection, command.VehicleID)
			if err != nil {
				return participants{}, err
			}
			planned.vehicles = append(planned.vehicles, command.VehicleID)
			if held != nil {
				planned.rentals = append(planned.rentals, held.ID)
				planned.users = append(planned.users, held.UserID)
			}
		}
		if command.RentalID != "" {
			target, err := rentalByID(ctx, s.pool, command.RentalID)
			if errors.Is(err, ErrRentalNotFound) {
				return participants{}, nil
			}
			if err != nil {
				return participants{}, err
			}
			planned.vehicles = append(planned.vehicles, target.VehicleID)
			planned.rentals = append(planned.rentals, target.ID)
			planned.users = append(planned.users, target.UserID)
		}
		planned.vehicles = sortedIdentifiers(planned.vehicles...)
		planned.rentals = sortedIdentifiers(planned.rentals...)
		planned.users = distinctUsers(planned.users)
		return planned, nil
	}
}

func (s *Service) demoWithin(
	ctx context.Context,
	tx pgx.Tx,
	moment time.Time,
	command DemoCommand,
) (Outcome, error) {
	if command.Kind == demoaction.SetNextPaymentOutcome {
		return s.setPaymentOutcome(ctx, moment, command)
	}
	return s.setVehicleState(ctx, tx, moment, command)
}

// setVehicleState applies a change to what a vehicle is or where it stands. Every kind begins the same
// way: the vehicle is read, its model is brought to the moment of the command, and the rental that
// holds it is read again as the model left it.
func (s *Service) setVehicleState(
	ctx context.Context,
	tx pgx.Tx,
	moment time.Time,
	command DemoCommand,
) (Outcome, error) {
	vehicle, err := s.vehicles.SimulatedVehicle(ctx, command.VehicleID)
	if errors.Is(err, fleet.ErrVehicleNotFound) {
		return refused(moment, Refusal{Kind: VehicleNotFound}), nil
	}
	if err != nil {
		return Outcome{}, err
	}
	held, err := liveRentalOf(ctx, s.pool, vehicleLiveRentalSelection, command.VehicleID)
	if err != nil {
		return Outcome{}, err
	}
	reconciled, err := s.reconcileRead(ctx, tx, moment, vehicle, held)
	if err != nil {
		return Outcome{}, err
	}
	if reconciled.Ended != nil {
		held = nil
	}

	refusal, err := demoRefusal(vehicle, held, command)
	if err != nil {
		return Outcome{}, err
	}
	if refusal != nil {
		return refused(moment, *refusal), nil
	}

	state, stored, err := s.restore(ctx, moment, vehicle)
	if err != nil {
		return Outcome{}, err
	}
	if !stored {
		// Reconciling a vehicle always stores what it reached, so a state that is still missing here
		// is a model this build cannot account for rather than a vehicle it has not met yet.
		return Outcome{}, simulation.ErrStateUnusable
	}
	changed := changeVehicle(state, command)
	if err = s.models.Save(ctx, vehicle.ID, changed); err != nil {
		return Outcome{}, err
	}
	if err = s.publishVehicleState(ctx, tx, vehicle, changed, command); err != nil {
		return Outcome{}, err
	}
	return Outcome{Moment: moment}, nil
}

// demoRefusal reports why a demonstration command does not apply to this vehicle as it stands, or nil
// when it does.
func demoRefusal(vehicle fleet.SimulatedVehicle, held *Rental, command DemoCommand) (*Refusal, error) {
	switch command.Kind {
	case demoaction.SetPosition:
		// A vehicle is moved by hand only when it is standing still: a booked or moving vehicle is
		// where the ride put it, and moving it would rewrite a journey that is under way.
		if held != nil && held.Stage != stage.Paused {
			return &Refusal{Kind: VehicleInUse}, nil
		}
	case demoaction.MarkServiced:
		// Servicing is what puts a vehicle nobody may book back into the fleet, so it applies to a
		// vehicle no rental holds.
		if held != nil {
			return &Refusal{Kind: VehicleInUse}, nil
		}
	case demoaction.SetEnergyRemaining:
		if !carriesSource(vehicle, command.Source) {
			return &Refusal{Kind: SourceNotCarried}, nil
		}
		if capacity := capacityOf(vehicle, command.Source); command.Remaining > capacity {
			return &Refusal{Kind: SourceCapacityExceeded}, nil
		}
	}
	return nil, nil
}

// changeVehicle is the model of a vehicle as one command leaves it. The state is returned rather than
// written here, so a kind that changes nothing about the model still passes through one place.
func changeVehicle(
	state simulation.State, command DemoCommand,
) simulation.State {
	switch command.Kind {
	case demoaction.SetPosition:
		return positioned(state, command.Position)
	case demoaction.SetEnergyRemaining:
		return refilled(state, command.Source, command.Remaining)
	case demoaction.MarkServiced:
		return serviced(state)
	default:
		// Linking and unlinking a vehicle changes nothing about where it is or what it holds.
		return state
	}
}

// publishVehicleState writes what a command changed into the fleet the catalog reads, raises the
// version of the vehicle and records the signal of the change.
//
// A vehicle that confirms its telemetry publishes where it stands and what it holds together, because
// that is what a confirmation is; one that does not confirm keeps the reading it last confirmed. A
// vehicle that has just been unlinked therefore keeps the position and the reserve a client already
// had, and the reading of it ages out of freshness on the ordinary rule.
func (s *Service) publishVehicleState(
	ctx context.Context,
	tx pgx.Tx,
	vehicle fleet.SimulatedVehicle,
	state simulation.State,
	command DemoCommand,
) error {
	version, err := s.vehicles.PublishChange(ctx, tx, vehicle.ID, pendingVehicleChange(command))
	if err != nil {
		return err
	}
	if confirming(vehicle, command) {
		if err = s.vehicles.Confirm(ctx, tx, confirmedVehicleState(vehicle.ID, state)); err != nil {
			return err
		}
	}
	return events.Record(ctx, s.pool, events.Signal{
		Kind:       events.VehicleChanged,
		ResourceID: vehicle.ID,
		Version:    version,
	})
}

// pendingVehicleChange is what a command states about a vehicle beyond its model: whether it is linked
// and whether it is out of service. A flag the command does not speak about is absent, which the
// statement reads as "leave it as it is".
func pendingVehicleChange(command DemoCommand) fleet.VehicleChange {
	var change fleet.VehicleChange
	if command.Kind == demoaction.SetTelemetryState {
		change.Connected = &command.Online
		change.Reporting = &command.Online
	}
	if command.Kind == demoaction.MarkServiced {
		serviceRequired := false
		change.ServiceRequired = &serviceRequired
	}
	return change
}

func confirmedVehicleState(vehicleID string, state simulation.State) fleet.Confirmation {
	confirmation := fleet.Confirmation{VehicleID: vehicleID, Position: state.Position}
	for _, source := range state.Sources {
		confirmation.Sources = append(confirmation.Sources, fleet.EnergySource{
			Kind:      source.Kind,
			Remaining: source.Remaining(),
		})
	}
	return confirmation
}

// confirming reports whether the vehicle confirms what it does after this command. Linking a vehicle
// makes it confirm again; unlinking it stops it; every other command leaves the link as it was.
func confirming(vehicle fleet.SimulatedVehicle, command DemoCommand) bool {
	if command.Kind == demoaction.SetTelemetryState {
		return command.Online
	}
	return vehicle.Confirming()
}

// setPaymentOutcome records what the next attempt at the payment of one ride decides. The demand is
// bound to the ride rather than to the installation, so two demonstrations running at once do not
// decide each other's outcome, and setting one for a ride that has not ended yet prepares the attempt
// its ending will owe.
func (s *Service) setPaymentOutcome(
	ctx context.Context, moment time.Time, command DemoCommand,
) (Outcome, error) {
	target, err := rentalByID(ctx, s.pool, command.RentalID)
	if errors.Is(err, ErrRentalNotFound) {
		return refused(moment, Refusal{Kind: RentalNotFound}), nil
	}
	if err != nil {
		return Outcome{}, err
	}
	if _, err = database.QuerierFrom(ctx, s.pool).Exec(ctx, recordPaymentDemandStatement,
		target.ID, command.Outcome, moment); err != nil {
		return Outcome{}, err
	}
	return Outcome{Rental: target, Moment: moment}, nil
}

// carriesSource reports whether the powertrain of a vehicle moves it on this kind of source. A source
// the profile does not carry cannot be refilled, because nothing would ever spend it.
func carriesSource(vehicle fleet.SimulatedVehicle, kind fleet.SourceKind) bool {
	profile, known := fleet.ProfileOf(vehicle.PowertrainType)
	if !known {
		return false
	}
	for _, carried := range profile.Sources {
		if carried == kind {
			return true
		}
	}
	return false
}

func capacityOf(vehicle fleet.SimulatedVehicle, kind fleet.SourceKind) fleet.Amount {
	for _, source := range vehicle.Sources {
		if source.Kind == kind {
			return source.Capacity
		}
	}
	return 0
}

// OnRouteToleranceMetres is how far a place may lie from a route and still count as being on it. A
// demonstration coordinate is written to about a metre, so a vehicle placed on its route is placed
// within the rounding of one.
const OnRouteToleranceMetres = 1.0

// positioned puts a vehicle where a command says it stands. A place on the route continues along it; a
// place beside it becomes the beginning of the connecting stretch the model drives back to the route,
// which is what keeps a hand-placed vehicle from being teleported onto its circuit.
func positioned(state simulation.State, at fleet.Position) simulation.State {
	route, declared := simulation.RouteOf(state.RouteID)
	if !declared {
		return state
	}
	join, distance := route.DistanceFrom(at)
	state.Position = at
	switch {
	case distance > OnRouteToleranceMetres:
		state.Path = 0
		state.JoinPath = join
		state.IsOffRoute = true
	default:
		state.Path = join
		state.JoinPath = 0
		state.IsOffRoute = false
	}
	return state
}

// refilled states what one source of a vehicle holds now. A refill of the source the profile uses
// first puts the vehicle back on it from this moment, and a vehicle that had run out can move again;
// neither of those revives a ride that has ended, and neither clears the service it requires.
func refilled(state simulation.State, kind fleet.SourceKind, remaining fleet.Amount) simulation.State {
	sources := make([]simulation.Source, 0, len(state.Sources))
	depleted := true
	for _, source := range state.Sources {
		if source.Kind == kind {
			source = simulation.SourceWith(source.Kind, source.Capacity, remaining)
		}
		if source.Remaining() > 0 {
			depleted = false
		}
		sources = append(sources, source)
	}
	state.Sources = sources
	state.Depleted = depleted
	return state
}

// serviced returns a vehicle to its service point with every source full: the beginning of its route,
// every source holding what it holds when it is full, and nothing left of what it had spent.
func serviced(state simulation.State) simulation.State {
	route, declared := simulation.RouteOf(state.RouteID)
	if declared {
		state.Path = 0
		state.JoinPath = 0
		state.IsOffRoute = false
		state.Position = route.PositionAt(0)
	}
	sources := make([]simulation.Source, 0, len(state.Sources))
	for _, source := range state.Sources {
		sources = append(sources, simulation.SourceWith(source.Kind, source.Capacity, source.Capacity))
	}
	state.Sources = sources
	state.Depleted = false
	return state
}
