package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FinishCommand ends one ride of one account, whoever asks. Whether the caller may end it is decided
// from the rental the module reads rather than from anything the request carries: the contract states
// that a finish uses confirmed server telemetry, so no coordinate, amount or owner travels with it.
type FinishCommand struct {
	Caller   uuid.UUID
	RentalID string
	Attempt  Attempt
}

// Finish ends the caller's ride and issues the invoice for it, or answers why it cannot.
//
// The whole ending is one transaction under the shared lock order: the account, the vehicle and the
// rental are locked, the relationships are read again under those locks, and the moment every boundary
// is judged by is read from the database afterwards. A ride another attempt has already ended is
// answered with that attempt's ending rather than written over, which is what makes two finishes of
// one ride produce one completed rental and one invoice.
func (s *Service) Finish(ctx context.Context, command FinishCommand) (Answered, error) {
	return s.answer(ctx, command.Caller, command.Attempt,
		rideParticipants(s.pool, RideCommand{Caller: command.Caller, RentalID: command.RentalID}),
		func(ctx context.Context, moment time.Time) (Outcome, error) {
			return s.finishWithin(ctx, moment, command)
		})
}

// finishWithin decides one finish with the participants locked and the moment fixed.
func (s *Service) finishWithin(
	ctx context.Context, moment time.Time, command FinishCommand,
) (Outcome, error) {
	target, err := rentalByIDFor(ctx, s.pool, command.Caller, command.RentalID)
	if errors.Is(err, ErrRentalNotFound) {
		return refused(moment, Refusal{Kind: RentalNotFound}), nil
	}
	if err != nil {
		return Outcome{}, err
	}

	// A ride that has already ended is not ended again: the reason and the moment the ending
	// transaction stored are what a new key is answered with, together with the invoice that
	// transaction issued. Neither of them is replaced.
	if target.Stage == stage.Completed {
		return s.storedEnding(ctx, moment, target)
	}
	if refusal := finishRefusal(target); refusal != nil {
		return refused(moment, *refusal), nil
	}

	refusal, err := s.finishPrepared(ctx, moment, target)
	if err != nil {
		return Outcome{}, err
	}
	if refusal != nil {
		return refused(moment, *refusal), nil
	}
	return s.endRide(ctx, moment, target)
}

// finishPrepared reports what a finish requires of the vehicle before it may end the ride. Nothing is
// written when it refuses: the ride keeps its mode, its open interval and its charging, exactly as the
// product states for an ending that is not admissible where the vehicle stands.
func (s *Service) finishPrepared(
	ctx context.Context, moment time.Time, target Rental,
) (*Refusal, error) {
	vehicle, err := s.vehicles.VehicleAt(ctx, target.VehicleID, moment)
	if err != nil {
		return nil, err
	}
	return finishLandingRefusal(vehicle, target.ZoneID, moment, s.finishLanding), nil
}

// storedEnding answers a ride that has already ended with what the ending transaction stored: the
// rental as it stands and the invoice of that ride.
func (s *Service) storedEnding(ctx context.Context, moment time.Time, target Rental) (Outcome, error) {
	issued, err := s.invoices.ByRental(ctx, target.ID)
	if err != nil {
		return Outcome{}, err
	}
	vehicle, err := s.vehicles.VehicleAt(ctx, target.VehicleID, moment)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Rental: target, Vehicle: vehicle, Moment: moment, Invoice: issued}, nil
}

// endRide closes the ride and writes everything the ending owes, in the transaction the command
// already holds: the interval that was open, the stage and the moment the ride ended, the reason it
// ended, the invoice of what it cost, the report of it and the signals of the changes it made.
func (s *Service) endRide(ctx context.Context, moment time.Time, target Rental) (Outcome, error) {
	ended, err := completeRental(ctx, s.pool, target, moment)
	if err != nil {
		return Outcome{}, err
	}
	if err = closeOpenSegment(ctx, s.pool, ended.ID, moment); err != nil {
		return Outcome{}, err
	}

	priced, err := priceRide(ctx, s.pool, ended)
	if err != nil {
		return Outcome{}, err
	}
	issued, err := s.invoices.Issue(ctx, invoiceDraft(ended, priced, moment))
	if err != nil {
		return Outcome{}, err
	}
	if err = s.completions.Record(ctx, ended.UserID, ended.ID, notifications.Completion{
		InvoiceID: issued.ID,
		Reason:    issued.Completion,
	}, moment); err != nil {
		return Outcome{}, err
	}
	if err = announceFinish(ctx, s.pool, ended, issued); err != nil {
		return Outcome{}, err
	}

	vehicle, err := s.vehicles.VehicleAt(ctx, ended.VehicleID, moment)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Rental: ended, Vehicle: vehicle, Moment: moment, Invoice: issued}, nil
}

// finishReason is why this build ends a ride when a person ends it. The other reason the contract
// publishes belongs to the transition that ends a ride the service decides to end.
const finishReason = completion.UserFinished

// completeRentalStatement writes the end of one ride. The stages it applies to are part of the
// statement, so a rental another transaction has ended is reported as unmoved rather than written
// over, and the moment the ending transaction fixed is the moment the ride ended. A ride that is over
// is in no mode, so it releases the moment its current mode began, exactly as an ended reservation
// does.
const completeRentalStatement = `
UPDATE rentals
SET stage = $2::text,
    ended_at = $3,
    mode_started_at = NULL,
    completion_reason = $4::text,
    version = version + 1
WHERE id = $1 AND stage = ANY($5::text[])
RETURNING` + rentalFields

// errRentalNotEnded reports a rental that no longer stood in a stage a finish applies to. The stage is
// read under the lock of the same transaction, so this is a defect of that reading rather than a
// refusal a client caused.
var errRentalNotEnded = errors.New("the rental was not a ride that could be ended")

// completeRental writes one ending. It is the only place a rental reaches the completed stage, so the
// reason, the moment and the release of the vehicle are written together or not at all.
func completeRental(
	ctx context.Context, pool *pgxpool.Pool, target Rental, moment time.Time,
) (Rental, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, completeRentalStatement,
		target.ID, string(stage.Completed), moment, string(finishReason), rideStages)
	if err != nil {
		return Rental{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Rental{}, err
		}
		return Rental{}, errRentalNotEnded
	}
	var ended Rental
	if err = scanRental(rows, &ended); err != nil {
		return Rental{}, err
	}
	return ended, rows.Err()
}

// rideStages are the stages a ride that has begun stands in, which is what a finish applies to: a
// reservation is given back or runs out rather than being finished.
var rideStages = []string{string(stage.Active), string(stage.Paused)}

// priceRide computes what a ride costs by the moment it ended. That moment is the one the ending
// stored rather than the moment the answer is given: reading the intervals to a later moment would
// count the time the transaction itself took, and an invoice must state what the ride took rather
// than how long the service spent writing it down.
func priceRide(ctx context.Context, pool *pgxpool.Pool, ended Rental) (billing.Charge, error) {
	if ended.EndedAt == nil {
		return billing.Charge{}, errRentalNotEnded
	}
	durations, err := readModeDurations(ctx, pool, ended.ID, *ended.EndedAt)
	if err != nil {
		return billing.Charge{}, err
	}
	return ended.priceOf(durations)
}

// invoiceDraft is what the invoices module is told about one finished ride. The lines are read from the
// charge in the unit an invoice stores, and the currency and the policy are copied from the rental by
// the statement, so a caller cannot hand an invoice a price list its ride never had.
func invoiceDraft(ended Rental, priced billing.Charge, moment time.Time) invoices.Draft {
	driving, paused := priced.Lines()
	return invoices.Draft{
		RentalID:   ended.ID,
		UserID:     ended.UserID,
		IssuedAt:   moment,
		Completion: finishReason,
		Driving:    invoices.LineOf(driving),
		Paused:     invoices.LineOf(paused),
		TotalTyiyn: priced.TotalTyiyn,
	}
}

// announceFinish records every change the ending made: the rental that reached its last stage, the
// vehicle that stopped being held and is therefore free again in the catalog, the invoice that was
// issued, and the work a worker must still deliver.
//
// The delivery of an invoice is the letter the contract describes and no process of this build sends
// yet. The task is recorded anyway: the queue keeps a kind whose delivery is not declared, so the
// letter is owed from the moment the ride ends rather than from the moment somebody remembers it.
func announceFinish(ctx context.Context, pool *pgxpool.Pool, ended Rental, issued invoices.Invoice) error {
	version, err := raiseVehicleVersion(ctx, pool, ended.VehicleID)
	if err != nil {
		return err
	}
	if err = events.Record(ctx, pool,
		events.Signal{Kind: events.RentalChanged, ResourceID: ended.ID,
			Version: ended.Version, Recipient: ended.UserID},
		events.Signal{Kind: events.VehicleChanged, ResourceID: ended.VehicleID, Version: version},
		events.Signal{Kind: events.InvoiceChanged, ResourceID: issued.ID,
			Version: issued.Version, Recipient: ended.UserID},
	); err != nil {
		return err
	}
	return events.Record(ctx, pool, events.Signal{
		Kind:       invoiceIssuedTask,
		ResourceID: issued.ID,
		Version:    ended.Version,
		Recipient:  ended.UserID,
	})
}

// invoiceIssuedTask names the durable work an ending owes: the letter with the invoice and the reason
// the ride ended. It is named here, beside the ending that records it, and the process that delivers
// it declares the same kind where the deliveries of the queue are assembled.
const invoiceIssuedTask events.Kind = "invoice.issued"
