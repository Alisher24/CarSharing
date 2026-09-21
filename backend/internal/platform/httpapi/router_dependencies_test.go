package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// An application assembled without one of its parts must fail where it is built, because the
// alternative is a handler that dereferences what it was not given and answers a client with a
// crash. The dependencies are filled in one at a time, so each step has exactly one absent and the
// error must name it.
func TestIncompleteApplicationIsRefusedAtConstruction(t *testing.T) {
	probe := func(context.Context) (servedapi.ReadyStatus, error) { return servedapi.ReadyStatus{}, nil }
	dependencies := Dependencies{Probe: probe}
	steps := []struct {
		absent string
		supply func(*Dependencies)
	}{
		{"database pool", func(d *Dependencies) { d.Pool = &pgxpool.Pool{} }},
		{"session manager", func(d *Dependencies) { d.Sessions = &sessions.Manager{} }},
		{"account service", func(d *Dependencies) { d.Auth = &auth.Service{} }},
		{"user store", func(d *Dependencies) { d.Users = &auth.UserStore{} }},
		{"rate-limit throttle", func(d *Dependencies) { d.Throttle = &auth.Throttle{} }},
		{"vehicle catalog", func(d *Dependencies) { d.Catalog.Vehicles = fixedCatalog{} }},
		{"service zones", func(d *Dependencies) { d.Catalog.Zones = zoneReader{} }},
		{"tariffs", func(d *Dependencies) { d.Catalog.Tariffs = tariffReader{} }},
		{"reservation commands", func(d *Dependencies) { d.Reservations = fixedReservations{} }},
		{"notification operations", func(d *Dependencies) { d.Notifications = fixedNotifications{} }},
		{"cursor signer", func(d *Dependencies) { d.Cursors = mustTestSigner(t) }},
		{"invoice reads", func(d *Dependencies) { d.Invoices = fixedInvoices{} }},
		{"event streams", func(d *Dependencies) { d.Events = fixedStreams{} }},
	}

	for _, step := range steps {
		t.Run(step.absent, func(t *testing.T) {
			handler, err := NewHandler(dependencies)
			if !errors.Is(err, ErrIncompleteApplication) {
				t.Fatalf("err = %v, want %v", err, ErrIncompleteApplication)
			}
			if !strings.Contains(err.Error(), step.absent) {
				t.Fatalf("err = %v, does not name %q", err, step.absent)
			}
			if handler != nil {
				t.Fatal("an incomplete application produced a handler")
			}
		})
		step.supply(&dependencies)
	}
}

// fixedReservations and fixedNotifications fill the two feature dependencies for a test that is
// about what the handler is missing rather than about what it answers: no case here reaches a
// method of either.
type fixedReservations struct{}

func (fixedReservations) Reserve(
	context.Context, rentals.ReserveCommand,
) (rentals.Answered, error) {
	return rentals.Answered{}, nil
}

func (fixedReservations) Cancel(context.Context, rentals.CancelCommand) (rentals.Answered, error) {
	return rentals.Answered{}, nil
}

func (fixedReservations) Current(context.Context, uuid.UUID) (rentals.Current, error) {
	return rentals.Current{}, nil
}

func (fixedReservations) Rides(
	context.Context, uuid.UUID, *rentals.RidePosition, int,
) (rentals.RidePage, error) {
	return rentals.RidePage{}, nil
}

func (fixedReservations) StartRide(
	context.Context, rentals.StartRideCommand,
) (rentals.Answered, error) {
	return rentals.Answered{}, nil
}

func (fixedReservations) PauseRide(
	context.Context, rentals.PauseRideCommand,
) (rentals.Answered, error) {
	return rentals.Answered{}, nil
}

func (fixedReservations) ResumeRide(
	context.Context, rentals.ResumeRideCommand,
) (rentals.Answered, error) {
	return rentals.Answered{}, nil
}

func (fixedReservations) FinishRide(
	context.Context, rentals.FinishCommand,
) (rentals.Answered, error) {
	return rentals.Answered{}, nil
}

func (fixedReservations) PayInvoice(
	context.Context, rentals.PayCommand,
) (rentals.Answered, error) {
	return rentals.Answered{}, nil
}

type fixedNotifications struct{}

func (fixedNotifications) Collection(
	context.Context, uuid.UUID, *notifications.Position, int,
) (notifications.Collection, error) {
	return notifications.Collection{}, nil
}

func (fixedNotifications) MarkRead(
	context.Context, uuid.UUID, string,
) (notifications.Result, error) {
	return notifications.Result{}, nil
}

type fixedInvoices struct{}

func (fixedInvoices) ByID(context.Context, uuid.UUID, string) (invoices.Invoice, error) {
	return invoices.Invoice{}, nil
}

func (fixedInvoices) ReadPage(
	context.Context, uuid.UUID, *invoices.Position, int,
) (invoices.Page, error) {
	return invoices.Page{}, nil
}

// mustTestSigner is a signer holding a key long enough to sign, which the dependency test only
// needs present: the cursors it would issue are never read.
func mustTestSigner(t *testing.T) *cursor.Signer {
	t.Helper()
	signer, err := cursor.NewSigner([]byte(strings.Repeat("k", cursor.MinKeyLength)))
	if err != nil {
		t.Fatal(err)
	}
	return signer
}
