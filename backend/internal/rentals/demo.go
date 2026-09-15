package rentals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DemoActionKind is one set-to-value change a demonstration may make. The spelling is the one the
// internal contract publishes as the action.
type DemoActionKind string

const (
	// SetTelemetryState links or unlinks a vehicle, which is what shows the difference between a
	// vehicle that reports and one that keeps the reading it last confirmed.
	SetTelemetryState DemoActionKind = "set_telemetry_state"

	// SetPosition puts a vehicle somewhere by hand. It is refused while the vehicle is booked or
	// moving, and the model drives it back to its route from wherever it was put.
	SetPosition DemoActionKind = "set_position"

	// SetEnergyRemaining states what one source of a vehicle holds now, which is how a demonstration
	// prepares a ride that runs out within minutes.
	SetEnergyRemaining DemoActionKind = "set_energy_remaining"

	// MarkServiced returns a vehicle to its service point, fills every source and clears the flag
	// that took it out of service.
	MarkServiced DemoActionKind = "mark_serviced"

	// SetNextPaymentOutcome records what the next attempt at the payment of one ride will decide.
	SetNextPaymentOutcome DemoActionKind = "set_next_payment_outcome"
)

// DemoCommand is one demonstration command. Which fields it carries follows from its kind: a command
// that names a source states no position, and one that names a rental states no vehicle. The fields a
// kind does not use are read by nothing.
type DemoCommand struct {
	ActionID string
	Kind     DemoActionKind
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
	case SetTelemetryState:
		return c.needsVehicle()
	case SetPosition:
		return c.needsVehicle()
	case SetEnergyRemaining:
		if err := c.needsVehicle(); err != nil {
			return err
		}
		if c.Source == "" {
			return errors.New("a refill must name the source it fills")
		}
		return nil
	case MarkServiced:
		return c.needsVehicle()
	case SetNextPaymentOutcome:
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
		func(ctx context.Context, moment time.Time) (Outcome, error) {
			return s.demoWithin(ctx, moment, command)
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
	ctx context.Context, moment time.Time, command DemoCommand,
) (Outcome, error) {
	if command.Kind == SetNextPaymentOutcome {
		return s.setPaymentOutcome(ctx, moment, command)
	}
	return s.setVehicleState(ctx, moment, command)
}

// setVehicleState applies a change to what a vehicle is or where it stands. Every kind begins the same
// way: the vehicle is read, its model is brought to the moment of the command, and the rental that
// holds it is read again as the model left it.
func (s *Service) setVehicleState(
	ctx context.Context, moment time.Time, command DemoCommand,
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
	reconciled, err := s.reconcileRead(ctx, moment, vehicle, held)
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
	if err = s.publishVehicleState(ctx, vehicle, changed, command); err != nil {
		return Outcome{}, err
	}
	return Outcome{Moment: moment}, nil
}

// demoRefusal reports why a demonstration command does not apply to this vehicle as it stands, or nil
// when it does.
func demoRefusal(vehicle fleet.SimulatedVehicle, held *Rental, command DemoCommand) (*Refusal, error) {
	switch command.Kind {
	case SetPosition:
		// A vehicle is moved by hand only when it is standing still: a booked or moving vehicle is
		// where the ride put it, and moving it would rewrite a journey that is under way.
		if held != nil && held.Stage != stage.Paused {
			return &Refusal{Kind: VehicleInUse}, nil
		}
	case MarkServiced:
		// Servicing is what puts a vehicle nobody may book back into the fleet, so it applies to a
		// vehicle no rental holds.
		if held != nil {
			return &Refusal{Kind: VehicleInUse}, nil
		}
	case SetEnergyRemaining:
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
	case SetPosition:
		return positioned(state, command.Position)
	case SetEnergyRemaining:
		return refilled(state, command.Source, command.Remaining)
	case MarkServiced:
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
	ctx context.Context, vehicle fleet.SimulatedVehicle, state simulation.State, command DemoCommand,
) error {
	connectivity, serviceRequired := pendingVehicleFlags(vehicle, command)
	version, err := publishDemoChange(ctx, s.pool, vehicle.ID, connectivity, serviceRequired)
	if err != nil {
		return err
	}
	if confirming(vehicle, command) {
		if err = confirmModel(ctx, s.pool, vehicle.ID, state); err != nil {
			return err
		}
	}
	return events.Record(ctx, s.pool, events.Signal{
		Kind:       events.VehicleChanged,
		ResourceID: vehicle.ID,
		Version:    version,
	})
}

// pendingVehicleFlags is what a command states about a vehicle beyond its model: whether it is linked
// and whether it is out of service. A flag the command does not speak about is absent, which the
// statement reads as "leave it as it is".
func pendingVehicleFlags(vehicle fleet.SimulatedVehicle, command DemoCommand) (any, any) {
	var connectivity any
	if command.Kind == SetTelemetryState {
		connectivity = command.Online
	}
	var serviceRequired any
	if command.Kind == MarkServiced {
		serviceRequired = false
	}
	return connectivity, serviceRequired
}

// confirming reports whether the vehicle confirms what it does after this command. Linking a vehicle
// makes it confirm again; unlinking it stops it; every other command leaves the link as it was.
func confirming(vehicle fleet.SimulatedVehicle, command DemoCommand) bool {
	if command.Kind == SetTelemetryState {
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

const recordPaymentDemandStatement = `
INSERT INTO demo_payment_outcomes (rental_id, outcome, set_at)
VALUES ($1, $2, $3)
ON CONFLICT (rental_id) DO UPDATE SET outcome = EXCLUDED.outcome, set_at = EXCLUDED.set_at`

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

// The statements a demonstration command uses to publish what it changed. A confirmation states where
// the vehicle stands and what it holds at one moment, which is why the moment is read from the
// database rather than taken from the command.
const (
	publishDemoChangeStatement = `
UPDATE vehicles
SET version = version + 1,
    connected = coalesce($2::boolean, connected),
    reporting = coalesce($2::boolean, reporting),
    service_required = coalesce($3::boolean, service_required)
WHERE id = $1
RETURNING version`

	confirmPositionStatement = `
UPDATE vehicle_telemetry
SET position = ST_SetSRID(ST_MakePoint($2, $3), $4),
    confirmed_at = clock_timestamp()
WHERE vehicle_id = $1`

	confirmSourceStatement = `
UPDATE vehicle_energy_sources
SET remaining = $3::numeric
WHERE vehicle_id = $1 AND source_kind = $2`
)

func publishDemoChange(
	ctx context.Context, pool *pgxpool.Pool, vehicleID string, connectivity, serviceRequired any,
) (int64, error) {
	var version int64
	err := database.QuerierFrom(ctx, pool).QueryRow(ctx, publishDemoChangeStatement,
		vehicleID, connectivity, serviceRequired).Scan(&version)
	return version, err
}

// confirmModel publishes what the model holds and where it stands as the vehicle's confirmed reading.
// It is what restores the difference between a vehicle that is linked and one that is not: a vehicle
// this runs for has said where it is, and one it does not keeps the reading it last confirmed.
func confirmModel(
	ctx context.Context, pool *pgxpool.Pool, vehicleID string, state simulation.State,
) error {
	querier := database.QuerierFrom(ctx, pool)
	if _, err := querier.Exec(ctx, confirmPositionStatement,
		vehicleID, state.Position.Longitude, state.Position.Latitude, fleet.WGS84SRID); err != nil {
		return err
	}
	for _, source := range state.Sources {
		if _, err := querier.Exec(ctx, confirmSourceStatement,
			vehicleID, string(source.Kind), source.Remaining().Decimal()); err != nil {
			return err
		}
	}
	return nil
}
