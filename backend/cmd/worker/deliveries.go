package main

import (
	"context"
	"net/http"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/mailstub"
	"github.com/Alisher24/CarSharing/backend/internal/outbox"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/jackc/pgx/v5/pgxpool"
)

// deliveries is the table of what this process delivers, assembled where the process is: the signals
// the queue carries to every API process, the first attempt at the payment of an invoice, and the
// letter that carries the invoice to the account that owes it.
//
// A task of a kind this table does not name is kept in the queue with its error, so a kind whose
// delivery belongs to a later task is owed rather than reported as delivered.
func deliveries(pool *pgxpool.Pool, letters config.InternalClient) (outbox.Deliveries, error) {
	payments, err := rentals.NewRentalPayment(pool)
	if err != nil {
		return nil, err
	}
	posted, err := mailstub.NewClient(letters.APIURL, letters.Token, http.DefaultTransport)
	if err != nil {
		return nil, err
	}
	delivery, err := mailstub.NewDelivery(invoices.NewStore(pool), auth.NewUserStore(pool), posted)
	if err != nil {
		return nil, err
	}
	table := events.Deliveries(pool)
	table[rentals.PaymentAttemptTask()] = func(ctx context.Context, task outbox.Task) error {
		_, err := payments.Attempt(ctx, task.ResourceID)
		return err
	}
	table[rentals.InvoiceIssuedTask()] = func(ctx context.Context, task outbox.Task) error {
		return delivery.Deliver(ctx, task.ResourceID)
	}
	return table, nil
}
