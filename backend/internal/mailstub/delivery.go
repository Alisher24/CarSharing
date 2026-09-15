package mailstub

import (
	"context"
	"errors"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
)

// Delivery delivers one letter about one invoice: it reads the invoice the task names, addresses the
// account that owes it, renders the letter from the invoice alone and hands it to the stub.
//
// It is a type of its own rather than a method of the box, because the process that delivers a task is
// the worker: it holds no HTTP request, reaches the stub over the network, and is built from what the
// queue and the configuration give it.
type Delivery struct {
	invoicing *invoices.Store
	accounts  *auth.UserStore
	letters   *Client
}

// ErrIncompleteDelivery refuses a delivery that was not given what it delivers with. A delivery that
// started without one would claim tasks it could never settle.
var ErrIncompleteDelivery = errors.New("the mail delivery is missing a dependency")

// NewDelivery assembles the delivery over the reads it needs and the client it sends with. All three
// are required and all three are assembled where the process is: a delivery that built its own reads
// would decide for itself which database it speaks to, and one that built its own client would decide
// which stub it delivers to.
func NewDelivery(invoicing *invoices.Store, accounts *auth.UserStore, letters *Client) (*Delivery, error) {
	switch {
	case invoicing == nil:
		return nil, ErrIncompleteDelivery
	case accounts == nil:
		return nil, ErrIncompleteDelivery
	case letters == nil:
		return nil, ErrIncompleteDelivery
	}
	return &Delivery{invoicing: invoicing, accounts: accounts, letters: letters}, nil
}

// Deliver performs one delivery and reports whether the letter reached the stub.
//
// An invoice the stub would be told about twice is delivered once: the key the client builds from the
// invoice is what makes the second delivery a repeat, and the stub answers it with the receipt of the
// first one.
//
// An invoice that is not there and an account that is not there are nothing to deliver rather than a
// failure: a task about a row that no longer exists has nothing to settle, and repeating it for ever
// would keep a queue busy with work nobody can ever finish.
func (d *Delivery) Deliver(ctx context.Context, invoiceID string) error {
	issued, _, err := d.invoicing.ByIDForRead(ctx, invoiceID)
	if errors.Is(err, invoices.ErrInvoiceNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	owner, err := d.accounts.ByID(ctx, issued.UserID)
	if errors.Is(err, auth.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// The letter states the invoice as the ending wrote it rather than as it stands now: it is rendered
	// again by every attempt that delivers it, and a sentence that followed a later payment would make
	// the second rendering a different letter under one key.
	letter := invoices.LetterOf(issued.AsIssued())
	_, err = d.letters.Deliver(ctx, issued.ID, Request{
		To:      string(owner.Email),
		Subject: letter.Subject,
		Text:    letter.Text,
	})
	return err
}
