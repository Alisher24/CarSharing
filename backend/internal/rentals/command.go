package rentals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrConcurrencyExhausted reports that a command could not be completed after starting over the
// permitted number of times. It is a refusal to keep trying rather than a report that nothing
// happened, and the caller may repeat the command with the same key to get a fresh decision.
var ErrConcurrencyExhausted = errors.New("the command could not be completed after repeated attempts")

// RefusalKind is the domain answer a command gave instead of moving a rental. Each one is a checked
// answer rather than a failure: it is stored with the command's key, so a repeat reproduces it.
type RefusalKind string

const (
	// VehicleUnavailable reports a vehicle that does not exist, that another rental holds, or that
	// does not meet the conditions the catalog states for starting.
	VehicleUnavailable RefusalKind = "vehicle_unavailable"

	// ActiveRentalExists reports that the account already holds a rental.
	ActiveRentalExists RefusalKind = "active_rental_exists"

	// DailyLimitReached reports that the account has already spent the day's free reservation.
	DailyLimitReached RefusalKind = "daily_limit_reached"

	// ReservationExpired reports a reservation whose deadline had already been reached, which the
	// command recorded before answering.
	ReservationExpired RefusalKind = "reservation_expired"

	// RentalCompleted reports a ride that has already finished, which no longer responds to the
	// commands that only apply to a reservation.
	RentalCompleted RefusalKind = "rental_completed"

	// InvalidRentalState reports a rental in a stage the command does not apply to.
	InvalidRentalState RefusalKind = "invalid_rental_state"

	// RentalNotFound reports an identifier this account holds no rental for, whether no such rental
	// exists or it belongs to somebody else.
	RentalNotFound RefusalKind = "rental_not_found"

	// OutsideServiceZone reports a ride whose confirmed position is not covered by the service area
	// the reservation was made in. The ride is not ended and keeps charging.
	OutsideServiceZone RefusalKind = "outside_service_zone"

	// TelemetryStale reports a ride whose vehicle has no confirmed position recent enough to decide
	// an ending by. The ride is not ended and keeps charging.
	TelemetryStale RefusalKind = "telemetry_stale"

	// OutstandingInvoice reports an account that owes money: a positive invoice of its own that
	// nothing has settled. No reservation is made, and the debt is cleared only by paying it.
	OutstandingInvoice RefusalKind = "outstanding_invoice"

	// PaymentInProgress reports an invoice whose first attempt the service still owes: a person waits
	// for that attempt rather than paying in its place, and the command that asked changes nothing.
	PaymentInProgress RefusalKind = "payment_in_progress"

	// InvoiceNotFound reports an identifier this account holds no invoice for, whether no such invoice
	// exists or it belongs to somebody else.
	InvoiceNotFound RefusalKind = "invoice_not_found"

	// VehicleInUse reports a vehicle a rental holds, which a demonstration may not move by hand or
	// service: a booked or moving vehicle is where the ride put it.
	VehicleInUse RefusalKind = "vehicle_in_use"

	// VehicleNotFound reports an identifier no vehicle this installation simulates carries, whether no
	// such vehicle exists or nothing placed it on a route.
	VehicleNotFound RefusalKind = "vehicle_not_found"

	// SourceNotCarried reports a source the powertrain of a vehicle does not move it on, which cannot
	// be refilled because nothing would ever spend it.
	SourceNotCarried RefusalKind = "source_not_carried"

	// SourceCapacityExceeded reports a reserve larger than the source of that vehicle holds when it is
	// full, which no vehicle can be given.
	SourceCapacityExceeded RefusalKind = "source_capacity_exceeded"
)

// Refusal is a domain answer that changed nothing, together with what displaying it needs.
type Refusal struct {
	Kind RefusalKind

	// UnavailableReasons is why the chosen vehicle could not be used, in the vocabulary the public
	// catalog publishes. It is empty for a refusal the catalog does not explain.
	UnavailableReasons []fleet.UnavailableReason

	// Limit is the state of the day's allowance at the moment of the command, which the refusal
	// that reports an exhausted allowance displays.
	Limit DailyLimit
}

// Outcome is what a command decided: the rental it moved, the vehicle as the answer publishes it at
// the moment the command fixed, that moment — or the refusal that changed nothing.
type Outcome struct {
	Rental  Rental
	Vehicle fleet.Vehicle
	Moment  time.Time

	// Progress is what a ride that has begun has taken by the moment of the answer. A command that
	// did not leave the rental a ride carries the zero value, which is not published.
	Progress billing.Charge

	// Invoice is what a finished ride cost. A command that issued no invoice carries the zero value,
	// which is not published.
	Invoice invoices.Invoice

	// Tick is what one call of the simulator changed. A command that is not a tick carries the zero
	// value, which is not published.
	Tick TickOutcome

	Refusal Refusal
}

// Refused reports whether the command decided nothing.
func (o Outcome) Refused() bool { return o.Refusal.Kind != "" }

// Response is one answer as the client receives it: the status and the encoded body.
type Response struct {
	Status int
	Body   []byte
}

// Render spells what a command decided as the answer the client receives. The transport layer
// supplies it, because the shape of an answer is the contract's business rather than this module's.
//
// The module calls it inside the transaction that makes the change and stores the bytes it produced
// with the command's key, so a repeat answers what the first attempt answered rather than a fresh
// rendering that a later change could contradict.
type Render func(Outcome) (Response, error)

// Attempt identifies one client attempt at a command: the key that makes a repeat answer the first
// answer, the fingerprint of what it asked for, and how its answer is spelled.
type Attempt struct {
	Key         idempotency.Key
	Fingerprint idempotency.Fingerprint
	Render      Render
}

// Answered is a command's answer, together with whether an earlier attempt already gave it.
type Answered struct {
	Response
	Replayed bool
}

// Service runs the reservation commands and answers what is current for one account. It is the
// module's runtime over one connection pool: every statement runs on the querier the context
// carries, so a command and the records it depends on commit together.
type Service struct {
	pool     *pgxpool.Pool
	vehicles *fleet.Store
	prices   *tariffs.Store

	// invoices records what a finished ride cost and recordCompletion reports it to the account that
	// rode. Both are reached inside the transaction that ends the ride, so neither can describe an
	// ending that was rolled back.
	invoices         *invoices.Store
	warnings         WarningOperations
	recordCompletion RecordCompletion

	// models is the simulated state of the fleet. A command reaches it inside its own transaction,
	// so the model a command advances is advanced with the change it makes rather than beside it.
	models *simulation.Store
}

// NewService assembles the module over one connection pool. Every dependency is named here and
// checked here, so a command never reaches a missing one and the process fails at startup instead.
func NewService(
	pool *pgxpool.Pool,
	vehicles *fleet.Store,
	prices *tariffs.Store,
	issued *invoices.Store,
	warnings WarningOperations,
	recordCompletion RecordCompletion,
	models *simulation.Store,
) (*Service, error) {
	for _, required := range []struct {
		name     string
		supplied bool
	}{
		{"database pool", pool != nil},
		{"vehicle catalog", vehicles != nil},
		{"price lists", prices != nil},
		{"invoice records", issued != nil},
		{"reservation warning operations", warnings.complete()},
		{"completion reports", recordCompletion != nil},
		{"simulated state", models != nil},
	} {
		if !required.supplied {
			return nil, fmt.Errorf("%w: %s", ErrIncompleteModule, required.name)
		}
	}
	return &Service{
		pool:             pool,
		vehicles:         vehicles,
		prices:           prices,
		invoices:         issued,
		warnings:         warnings,
		recordCompletion: recordCompletion,
		models:           models,
	}, nil
}

// ErrIncompleteModule refuses to serve a module whose dependencies were not all supplied. A command
// that reached a missing one would fail on the first request instead of at startup.
var ErrIncompleteModule = errors.New("the rentals module is missing a dependency")

// participants is every row one rental transaction will touch, worked out from an unlocked read
// before the first lock: the accounts it belongs to, the vehicles it concerns, and the rentals that
// currently hold them.
type participants struct {
	users    []uuid.UUID
	vehicles []string
	rentals  []string
}

type participantLock struct {
	name        string
	statement   string
	identifiers any
	empty       bool
}

// locks is the single declaration of the row-lock order shared by every rental transaction.
func (p participants) locks() []participantLock {
	return []participantLock{
		{name: "users", statement: lockUsersStatement, identifiers: p.users, empty: len(p.users) == 0},
		{name: "vehicles", statement: lockVehiclesStatement, identifiers: p.vehicles, empty: len(p.vehicles) == 0},
		{name: "rentals", statement: lockRentalsStatement, identifiers: p.rentals, empty: len(p.rentals) == 0},
	}
}

// sameRows reports whether two plans describe the same rows. The reads that produce them are
// ordered, so comparing them is comparing the relationships they found.
func (p participants) sameRows(other participants) bool {
	return sameOrder(p.users, other.users) &&
		sameOrder(p.vehicles, other.vehicles) &&
		sameOrder(p.rentals, other.rentals)
}

func sameOrder[T comparable](left, right []T) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// maxTransactionAttempts bounds how many times one command starts over. A transaction that keeps
// meeting a new participant is refused rather than retried for ever, and the refusal is repeatable:
// the client may send the same command again.
const maxTransactionAttempts = 5

// errParticipantsChanged reports that the relationships a transaction planned to touch are no longer
// the ones it locked. The transaction is started over rather than taking the missing lock at the
// end, which is what keeps the lock order the one above.
var errParticipantsChanged = errors.New("the relationships changed while the transaction was starting")

// transact runs one attempt at a rental transaction under the shared lock order.
//
// discover reads the relationships the command intends to touch, without locking anything, and is
// read a second time once the locks are taken. work runs with the locks held and the moment fixed.
func transact(
	ctx context.Context,
	pool *pgxpool.Pool,
	discover func(context.Context) (participants, error),
	work func(context.Context, pgx.Tx, time.Time) error,
) error {
	var last error
	for attempt := 1; attempt <= maxTransactionAttempts; attempt++ {
		last = database.InTransactionWithHandle(ctx, pool, func(txCtx context.Context, tx pgx.Tx) error {
			planned, err := discover(txCtx)
			if err != nil {
				return err
			}
			if err = lock(txCtx, tx, planned); err != nil {
				return err
			}
			locked, err := discover(txCtx)
			if err != nil {
				return err
			}
			if !planned.sameRows(locked) {
				return errParticipantsChanged
			}
			moment, err := database.Moment(txCtx, tx)
			if err != nil {
				return err
			}
			return work(txCtx, tx, moment)
		})
		if last == nil {
			return nil
		}
		if !restartable(last) {
			return last
		}
	}
	return fmt.Errorf("%w: %s", ErrConcurrencyExhausted, last)
}

// rentalParticipants is the rows the transition of one named rental touches: the account that holds
// it, its vehicle and the rental itself. Nothing here reads a second rental, so the transaction that
// takes these rows cannot meet a set of relationships it did not plan for.
func rentalParticipants(pool *pgxpool.Pool, id string) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		target, err := rentalByID(ctx, pool, id)
		if errors.Is(err, ErrRentalNotFound) {
			return participants{}, nil
		}
		if err != nil {
			return participants{}, err
		}
		return participants{
			users:    []uuid.UUID{target.UserID},
			vehicles: []string{target.VehicleID},
			rentals:  []string{target.ID},
		}, nil
	}
}

// lock takes the planned rows in the shared order. Each selection is ordered, so two transactions
// reaching the same set wait in the same sequence.
func lock(ctx context.Context, tx pgx.Tx, planned participants) error {
	for _, target := range planned.locks() {
		if target.empty {
			continue
		}
		if _, err := tx.Exec(ctx, target.statement, target.identifiers); err != nil {
			return err
		}
	}
	return nil
}

// restartable reports whether a failure is one the transaction may start over from. A deadlock or a
// serialization failure is the database telling this transaction it lost a race it can retry, and a
// changed participant set is the module telling itself the same thing.
func restartable(err error) bool {
	if errors.Is(err, errParticipantsChanged) {
		return true
	}
	var failure *pgconn.PgError
	if !errors.As(err, &failure) {
		return false
	}
	return failure.Code == codeDeadlockDetected || failure.Code == codeSerializationFailure
}

// The SQLSTATE codes a transaction may start over from.
const (
	codeDeadlockDetected     = "40P01"
	codeSerializationFailure = "40001"
)
