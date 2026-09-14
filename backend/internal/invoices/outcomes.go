package invoices

import (
	"context"
	"errors"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DemoOutcome is the outcome a demonstration asked the next attempt at one ride for. The two values are
// the whole of the vocabulary, and the table states the same two words.
type DemoOutcome string

const (
	// DemoPaid asks the attempt to succeed, which is also what no demand at all produces. It is a
	// value rather than an absence because setting it cancels a demand for a refusal.
	DemoPaid DemoOutcome = "paid"

	// DemoFailed asks the attempt to be refused, which is the one way a demonstration can reach the
	// state a person then repeats.
	DemoFailed DemoOutcome = "failed"
)

// Known reports whether an outcome is one the table admits, which is what a demand read from storage
// is judged by before it decides an attempt.
func (o DemoOutcome) Known() bool { return o == DemoPaid || o == DemoFailed }

// Outcomes is the table of one-shot demands for the outcome of the next payment attempt at a ride.
//
// The application reads a demand and spends it and cannot make one: the privilege the migration grants
// is SELECT and DELETE alone. Only the demonstration control writes this table, as the role that owns
// the schema, which is what makes the scenario protected rather than merely undocumented.
type Outcomes struct{ pool *pgxpool.Pool }

func NewOutcomes(pool *pgxpool.Pool) *Outcomes { return &Outcomes{pool: pool} }

// Consume reads the demand recorded for one ride and removes it in the same statement, so the attempt
// that reads it is the one that spends it: two attempts at one ride cannot both be decided by one
// demand, and a transaction that rolls back leaves the demand for the attempt that follows.
//
// It reports whether a demand was there at all. A ride nobody asked anything about is answered as no
// demand rather than as a demand for success: the two produce the same attempt, but they are not the
// same fact, and only one of them was asked for.
func (o *Outcomes) Consume(ctx context.Context, rentalID string) (DemoOutcome, bool, error) {
	var outcome DemoOutcome
	err := database.QuerierFrom(ctx, o.pool).
		QueryRow(ctx, consumeOutcomeStatement, rentalID).Scan(&outcome)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return outcome, true, nil
}

// consumeOutcomeStatement reads one demand and deletes it, which is the whole of "asked once". Reading
// and deleting in one statement is what ties them together: a demand deleted by a transaction that
// then rolled back is still in place, and a demand read without being deleted would decide the attempt
// after this one as well.
const consumeOutcomeStatement = `
DELETE FROM demo_payment_outcomes
WHERE rental_id = $1
RETURNING outcome`
