package rentals

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReserveCommand asks to hold one vehicle for one account.
type ReserveCommand struct {
	Caller    uuid.UUID
	VehicleID string
	Attempt   Attempt
}

// Reserve holds one available vehicle for the reservation lifetime, or answers why it cannot.
//
// The whole decision is one transaction: the accounts and vehicles involved are locked in the shared
// order, the relationships are read again under those locks, and only then are the day's allowance,
// the caller's own rental and the state of the chosen vehicle judged. A command that decides nothing
// still commits its answer, because the refusal is what a repeat of the same command must reproduce.
func (s *Service) Reserve(ctx context.Context, command ReserveCommand) (Answered, error) {
	return s.answer(ctx, command.Caller, command.Attempt,
		reserveParticipants(s.pool, command),
		func(ctx context.Context, moment time.Time) (Outcome, error) {
			return s.reservationWithin(ctx, moment, command)
		})
}

// answer runs one command: it either replays the answer the key already holds, or decides the
// command, renders its answer and stores that answer under the key. The claim, the decision and the
// stored answer are one transaction, so a change that is committed is a change whose answer can
// never be lost, and an answer that is stored describes a change that happened.
func (s *Service) answer(
	ctx context.Context,
	caller uuid.UUID,
	attempt Attempt,
	discover func(context.Context) (participants, error),
	decide func(context.Context, time.Time) (Outcome, error),
) (Answered, error) {
	var answered Answered
	err := transact(ctx, s.pool, discover, func(txCtx context.Context, moment time.Time) error {
		claim, err := s.results.Claim(txCtx, caller, attempt.Key, attempt.Fingerprint)
		if err != nil {
			return err
		}
		if !claim.Owns {
			answered = Answered{Response: Response(claim.Answered), Replayed: true}
			return nil
		}

		decided, err := decide(txCtx, moment)
		if err != nil {
			return err
		}
		response, err := attempt.Render(decided)
		if err != nil {
			return err
		}
		stored := idempotency.Result{Status: response.Status, Body: response.Body}
		if err := s.results.Complete(txCtx, caller, attempt.Key, stored); err != nil {
			return err
		}
		answered = Answered{Response: response}
		return nil
	})
	if err != nil {
		return Answered{}, err
	}
	return answered, nil
}

// reserveParticipants is the rows a reservation touches: the caller, the chosen vehicle, the holder
// of that vehicle when somebody has it, and the rentals that currently hold either of them.
func reserveParticipants(pool *pgxpool.Pool, command ReserveCommand) func(context.Context) (participants, error) {
	return func(ctx context.Context) (participants, error) {
		planned := participants{
			users:    []uuid.UUID{command.Caller},
			vehicles: []string{command.VehicleID},
		}
		held, err := liveRentalOf(ctx, pool, userLiveRentalSelection, command.Caller)
		if err != nil {
			return participants{}, err
		}
		taken, err := liveRentalOf(ctx, pool, vehicleLiveRentalSelection, command.VehicleID)
		if err != nil {
			return participants{}, err
		}
		if taken != nil {
			planned.users = append(planned.users, taken.UserID)
		}
		planned.users = distinctUsers(planned.users)
		planned.rentals = sortedIdentifiers(identifierOf(held), identifierOf(taken))
		return planned, nil
	}
}

// reservationWithin decides the command with the participants locked and the moment fixed.
func (s *Service) reservationWithin(
	ctx context.Context, moment time.Time, command ReserveCommand,
) (Outcome, error) {
	// An overdue reservation is not live for any path, so the two this command meets are released
	// before anything is judged: the one holding the caller and the one holding the chosen vehicle.
	// Both releases happen in this transaction, which is what lets a vehicle the fleet has not swept
	// yet be taken here and now — and why the refusals below do not undo them, because the answer is
	// committed with the transitions it describes.
	held, err := liveRentalAt(ctx, s.pool, moment, userLiveRentalSelection, command.Caller)
	if err != nil {
		return Outcome{}, err
	}
	if _, err = liveRentalAt(ctx, s.pool, moment, vehicleLiveRentalSelection, command.VehicleID); err != nil {
		return Outcome{}, err
	}

	// The day's allowance is judged first of what remains, because it is what a person must be told
	// about: their own live rental is a consequence of having spent it, and a client told only about
	// the rental would offer the command again as soon as that rental ended.
	limit, err := readDailyLimit(ctx, s.pool, command.Caller, moment)
	if err != nil {
		return Outcome{}, err
	}
	if !limit.Available {
		return refused(moment, Refusal{Kind: DailyLimitReached, Limit: limit}), nil
	}

	if held != nil {
		return refused(moment, Refusal{Kind: ActiveRentalExists}), nil
	}

	// A debt is judged next, before anything about the vehicle is read: it is the one condition here
	// a person clears themselves, and a client told about a vehicle it cannot have would offer the
	// command again the moment the vehicle was free.
	owed, err := s.invoices.Outstanding(ctx, command.Caller)
	if err != nil {
		return Outcome{}, err
	}
	if owed {
		return refused(moment, Refusal{Kind: OutstandingInvoice}), nil
	}

	vehicle, err := s.vehicles.VehicleAt(ctx, command.VehicleID, moment)
	if errors.Is(err, fleet.ErrVehicleNotFound) {
		return refused(moment, Refusal{Kind: VehicleUnavailable}), nil
	}
	if err != nil {
		return Outcome{}, err
	}
	if refusal := vehicleRefusal(vehicle, moment); refusal != nil {
		return refused(moment, *refusal), nil
	}

	price, err := s.prices.InForce(ctx)
	if err != nil {
		return Outcome{}, err
	}
	rental, reserved, err := insertReservation(ctx, s.pool, reservation{
		userID:    command.Caller,
		vehicleID: command.VehicleID,
		zoneID:    vehicle.ServiceZoneID,
		price:     price,
		moment:    moment,
	})
	if err != nil {
		return Outcome{}, err
	}
	if !reserved {
		// The indexes that allow one live rental of a vehicle and one of a person refused the row
		// after the locks were taken. Which of the two it was decides the answer, and nothing was
		// written either way.
		return refused(moment, contendedRefusal(ctx, s.pool, command)), nil
	}

	raised, err := raiseVehicleVersion(ctx, s.pool, command.VehicleID)
	if err != nil {
		return Outcome{}, err
	}
	if err = events.Record(ctx, s.pool,
		events.Signal{Kind: events.RentalChanged, ResourceID: rental.ID,
			Version: rental.Version, Recipient: command.Caller},
		events.Signal{Kind: events.VehicleChanged, ResourceID: command.VehicleID, Version: raised},
	); err != nil {
		return Outcome{}, err
	}
	// The vehicle is read again rather than reused: the answer publishes it as it now stands, held
	// by the reservation that has just been made and carrying the version that change reached.
	published, err := s.vehicles.VehicleAt(ctx, command.VehicleID, moment)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Rental: rental, Vehicle: published, Moment: moment}, nil
}

// vehicleRefusal reports why a vehicle may not be reserved at an instant, or nil when it may. A
// vehicle somebody holds is refused before its own condition is judged: how full it is says nothing
// about whether it is free.
func vehicleRefusal(vehicle fleet.Vehicle, moment time.Time) *Refusal {
	if vehicle.HeldBy.Holds() {
		return &Refusal{Kind: VehicleUnavailable}
	}
	state := vehicle.StateAt(moment)
	if state.Status != fleet.Available {
		return &Refusal{
			Kind:               VehicleUnavailable,
			UnavailableReasons: state.UnavailableReasons,
		}
	}
	return nil
}

// contendedRefusal names which live rental kept the row out: the caller's own or the vehicle's.
func contendedRefusal(ctx context.Context, pool *pgxpool.Pool, command ReserveCommand) Refusal {
	held, err := liveRentalOf(ctx, pool, userLiveRentalSelection, command.Caller)
	if err == nil && held != nil {
		return Refusal{Kind: ActiveRentalExists}
	}
	return Refusal{Kind: VehicleUnavailable}
}

func refused(moment time.Time, refusal Refusal) Outcome {
	return Outcome{Moment: moment, Refusal: refusal}
}

// reservation is one reservation about to be written.
type reservation struct {
	userID    uuid.UUID
	vehicleID string
	zoneID    string
	price     tariffs.Tariff
	moment    time.Time
}

// insertReservation writes the reservation together with the conditions it was made under and the
// deadline it was given. The deadline is written once, here: repeating the command, restarting the
// process and reading the rental again all report the moment that was stored.
//
// The insert does not fail on a conflicting live rental: it writes nothing and says so, because a
// refusal must leave this transaction alive long enough to store the refusal itself.
const insertReservationStatement = `
INSERT INTO rentals (
    id,
    user_id,
    vehicle_id,
    stage,
    tariff_id,
    zone_id,
    reserved_at,
    expires_at,
    tariff_currency,
    tariff_billing_policy,
    tariff_driving_rate_tyiyn_per_started_minute,
    tariff_paused_rate_tyiyn_per_started_minute,
    tariff_version,
    version
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT DO NOTHING`

const reservationInitialVersion = 1

func insertReservation(
	ctx context.Context, pool *pgxpool.Pool, about reservation,
) (Rental, bool, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Rental{}, false, err
	}
	written, err := database.QuerierFrom(ctx, pool).Exec(ctx, insertReservationStatement,
		id.String(),
		about.userID,
		about.vehicleID,
		stage.Reserved,
		about.price.ID,
		about.zoneID,
		about.moment,
		about.moment.Add(ReservationLifetime),
		about.price.Currency,
		about.price.BillingPolicy,
		about.price.DrivingRateTyiynPerStartedMinute,
		about.price.PausedRateTyiynPerStartedMinute,
		about.price.Version,
		reservationInitialVersion,
	)
	if err != nil {
		return Rental{}, false, err
	}
	if written.RowsAffected() == 0 {
		return Rental{}, false, nil
	}
	rental, err := rentalByID(ctx, pool, id.String())
	return rental, err == nil, err
}

// distinctUsers orders the accounts of a transaction the way the lock statement reads them.
func distinctUsers(users []uuid.UUID) []uuid.UUID {
	sort.Slice(users, func(left, right int) bool {
		return users[left].String() < users[right].String()
	})
	return withoutRepeats(users)
}

// sortedIdentifiers orders the rental identifiers of one transaction, leaving out the rentals that
// are absent. The lock statement reads them in this order, so two transactions touching the same
// rentals wait in the same sequence.
func sortedIdentifiers(identifiers ...string) []string {
	kept := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		if identifier != "" {
			kept = append(kept, identifier)
		}
	}
	sort.Strings(kept)
	return withoutRepeats(kept)
}

// identifierOf names a rental that may be absent, so a caller can list the rentals of a transaction
// without testing each one.
func identifierOf(rental *Rental) string {
	if rental == nil {
		return ""
	}
	return rental.ID
}

// withoutRepeats drops the neighbours that are equal. It requires ordered input, so repeats are
// adjacent, and reuses the backing array because the ordered slice is not read afterwards.
func withoutRepeats[T comparable](rows []T) []T {
	unique := rows[:0]
	for _, row := range rows {
		if len(unique) > 0 && unique[len(unique)-1] == row {
			continue
		}
		unique = append(unique, row)
	}
	return unique
}
