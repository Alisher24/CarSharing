package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ending is a ride that is over: why it ended, the moment it ended at, and, for a ride whose sources
// ran out, the sources that were empty when it did.
//
// The reason and the moment are read by one transaction: a command and a model that ran the same ride
// out cannot disagree about either, and a command that arrives late states the moment the ride really
// ended rather than the moment it was noticed.
type Ending struct {
	Reason    completion.Reason
	EndedAt   time.Time
	Exhausted []fleet.SourceKind
}

// Depleted is the ending a ride owes when the model can move it no further: every source that was
// empty at that moment, in the order the vehicle's profile uses them.
func Depleted(at time.Time, exhausted []fleet.SourceKind) Ending {
	return Ending{Reason: completion.EnergyDepleted, EndedAt: at, Exhausted: exhausted}
}

// endRide closes a ride and writes everything the ending owes, in the transaction the caller already
// holds: the interval that was open, the stage and the moment the ride ended, the reason it ended, the
// invoice of what it cost, the report of it and the signals of the changes it made.
//
// moment is the moment of the operation, which the invoice and the report are written at, and the
// ending's own moment is the moment the ride ended. The two differ exactly when a ride ran out before
// anything noticed: the ride is priced to the moment it ended, and the records of it are written at
// the moment they were written.
func (s *Service) endRide(
	ctx context.Context,
	tx pgx.Tx,
	moment time.Time,
	target Rental,
	ending Ending,
) (Outcome, error) {
	ended, err := completeRental(ctx, s.pool, target, ending)
	if err != nil {
		return Outcome{}, err
	}
	if err = closeOpenSegment(ctx, s.pool, ended.ID, ending.EndedAt); err != nil {
		return Outcome{}, err
	}

	priced, err := priceRide(ctx, s.pool, ended)
	if err != nil {
		return Outcome{}, err
	}
	issued, err := s.invoices.Issue(ctx, invoiceDraft(ended, priced, ending, moment))
	if err != nil {
		return Outcome{}, err
	}
	if err = s.recordCompletion(ctx, ended.UserID, ended.ID, CompletionReport{
		InvoiceID: issued.ID,
		Reason:    issued.Completion,
		Exhausted: ending.Exhausted,
		EndedAt:   ending.EndedAt,
	}, moment); err != nil {
		return Outcome{}, err
	}
	if err = announceEnding(ctx, s.pool, s.vehicles, tx, ended, issued, ending); err != nil {
		return Outcome{}, err
	}

	vehicle, err := s.vehicles.VehicleAt(ctx, ended.VehicleID, moment)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Rental: ended, Vehicle: vehicle, Moment: moment, Invoice: issued}, nil
}

// errRentalNotEnded reports a rental that no longer stood in a stage a finish applies to. The stage is
// read under the lock of the same transaction, so this is a defect of that reading rather than a
// refusal a client caused.
var errRentalNotEnded = errors.New("the rental was not a ride that could be ended")

// rideStages are the stages a ride that has begun stands in, which is what an ending applies to: a
// reservation is given back or runs out rather than being ended.
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
// the statement, so a caller cannot hand an invoice a price list its ride never had. The moment the
// invoice is issued at is the moment of the operation, which is not the moment the ride ended when a
// ride that had already run out was found later.
func invoiceDraft(ended Rental, priced billing.Charge, ending Ending, moment time.Time) invoices.Draft {
	driving, paused := priced.Lines()
	return invoices.Draft{
		RentalID:   ended.ID,
		UserID:     ended.UserID,
		IssuedAt:   moment,
		Completion: ending.Reason,
		Exhausted:  ending.Exhausted,
		Driving:    invoices.LineOf(driving),
		Paused:     invoices.LineOf(paused),
		TotalTyiyn: priced.TotalTyiyn,
	}
}

// announceEnding records every change the ending made: the rental that reached its last stage, the
// vehicle that stopped being held and is therefore free again in the catalog, the invoice that was
// issued, and the work a worker must still deliver.
//
// A ride whose sources ran out takes its vehicle out of service in the same transaction: the vehicle
// is not merely free again, it is one nobody may book until it has been looked at.
//
// The deliveries an ending owes are two: the attempt at the payment of the invoice and the letter that
// carries it. The attempt is recorded only for an invoice that is still waiting for one вЂ” a ride that
// cost nothing is settled by the moment its invoice was issued вЂ” and that question is asked of the
// state the invoice was stored with rather than of a second comparison of its amount with zero.
func announceEnding(
	ctx context.Context,
	pool *pgxpool.Pool,
	vehicles *fleet.Store,
	tx pgx.Tx,
	ended Rental,
	issued invoices.Invoice,
	ending Ending,
) error {
	change := fleet.VehicleChange{}
	if ending.Reason == completion.EnergyDepleted {
		serviceRequired := true
		change.ServiceRequired = &serviceRequired
	}
	version, err := vehicles.PublishChange(ctx, tx, ended.VehicleID, change)
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
	if err = recordPaymentAttempt(ctx, pool, ended, issued); err != nil {
		return err
	}
	return events.Record(ctx, pool, events.Signal{
		Kind:       invoiceIssuedTask,
		ResourceID: issued.ID,
		Version:    ended.Version,
		Recipient:  ended.UserID,
	})
}

// recordPaymentAttempt owes one attempt at the invoice of a ride that has ended, unless nothing is owed
// on it. The task names the invoice as its resource and the ride as the version it was recorded at, and
// it is addressed to the account that rode: the attempt belongs to one payment rather than to the
// installation.
func recordPaymentAttempt(
	ctx context.Context, pool *pgxpool.Pool, ended Rental, issued invoices.Invoice,
) error {
	if issued.Payment != invoices.PendingPayment {
		return nil
	}
	return events.Record(ctx, pool, events.Signal{
		Kind:       paymentAttemptTask,
		ResourceID: issued.ID,
		Version:    ended.Version,
		Recipient:  ended.UserID,
	})
}

// invoiceIssuedTask names the durable work an ending owes: the letter with the invoice and the reason
// the ride ended. It is named here, beside the ending that records it, and the process that delivers
// it declares the same kind where the deliveries of the queue are assembled.
const invoiceIssuedTask events.Kind = "invoice.issued"

// InvoiceIssuedTask is the kind of task the letter with one invoice is delivered as, for the
// composition root that names it in the table of deliveries. The kind is stated once, beside the
// ending that records it, because the module that owes the work and the process that performs it have
// to say the same word.
func InvoiceIssuedTask() string { return string(invoiceIssuedTask) }
