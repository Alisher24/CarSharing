package mailstub

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Receipt is what one delivery produced: the letter the key now holds, whether this delivery found it
// already stored, and whether this particular answer is the one a demonstration asked to lose.
type Receipt struct {
	Message  Message
	Replayed bool
	Dropped  bool
}

// Acceptor accepts one letter on behalf of the delivery operation.
//
// Storing the letter and spending the demand for the loss of its answer are one transaction, so a
// letter is either stored together with the demand it spent or not stored at all: a delivery that
// failed leaves the demand armed for the delivery that follows, and no delivery can spend a demand
// without the letter it decided reaching the box.
type Acceptor struct {
	pool   *pgxpool.Pool
	store  *Store
	faults *Faults
}

// NewAcceptor assembles the acceptance over one connection pool, which is the querier its statements
// and its transaction both run on.
func NewAcceptor(pool *pgxpool.Pool) (*Acceptor, error) {
	if pool == nil {
		return nil, errors.New("the mail box must be given a database pool")
	}
	return &Acceptor{pool: pool, store: NewStore(pool), faults: NewFaults(pool)}, nil
}

// momentStatement reads the moment one acceptance is written at. The database states it rather than
// the process, so a letter and the ride that produced it are dated by one clock.
const momentStatement = `SELECT clock_timestamp()`

// Accept stores one letter under its key and answers what the delivery produced. A letter the key
// already holds is answered as it stands, which is what makes a repeated delivery a repeat rather
// than a second letter.
//
// The demand for a lost answer belongs to the delivery that stores a letter: a repeat answers the
// receipt of the delivery that really arrived, and spending the demand on it would take the loss
// away from the delivery that is still to come — and, worse, lose an answer a second time for a key
// whose loss was already spent.
func (a *Acceptor) Accept(ctx context.Context, key Key, request Request) (Receipt, error) {
	var receipt Receipt
	err := database.InTransaction(ctx, a.pool, func(ctx context.Context) error {
		var moment time.Time
		if err := database.QuerierFrom(ctx, a.pool).QueryRow(ctx, momentStatement).Scan(&moment); err != nil {
			return err
		}
		stored, written, err := a.store.Accept(ctx, key, request, moment)
		if err != nil {
			return err
		}
		if !written {
			receipt = Receipt{Message: stored, Replayed: true}
			return nil
		}
		armed, err := a.faults.Consume(ctx)
		if err != nil {
			return err
		}
		receipt = Receipt{Message: stored, Dropped: armed}
		return nil
	})
	if err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}
