package mailstub

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Faults is the demonstration demand for the loss of the next answer the stub produces.
//
// It is the reason the stub exists in this build: a letter that was stored while its answer was lost
// is what proves the delivery is idempotent at the receiver rather than only in the queue that
// carries it. The demand decides exactly one delivery, and the process that serves a delivery can
// spend it but cannot make one.
type Faults struct{ pool *pgxpool.Pool }

func NewFaults(pool *pgxpool.Pool) *Faults { return &Faults{pool: pool} }

// consumeStatement reads the demand and removes it in one statement, so the delivery that reads it is
// the one it decides: two deliveries racing for one demand cannot both lose their answer, and a
// transaction that rolls back leaves the demand for the delivery that follows.
const consumeStatement = `
DELETE FROM mailstub.demo_delivery_faults
RETURNING armed_at`

// Consume spends the demand and reports whether one was armed. A demand that is not there is an
// ordinary delivery rather than a failure.
func (f *Faults) Consume(ctx context.Context) (bool, error) {
	var armedAt time.Time
	err := database.QuerierFrom(ctx, f.pool).QueryRow(ctx, consumeStatement).Scan(&armedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// armStatement calls the function the schema owner declares, which is the only way this role writes
// the demand: the demonstration asks for the loss, and the role that owns the schema is the one that
// decides the demand may be made at all.
const armStatement = `SELECT mailstub.arm_delivery_fault()`

// Arm records the demand for the loss of the next answer, replacing a demand that was already armed:
// a demonstration asks for the next delivery to lose its answer, whenever that delivery happens.
func (f *Faults) Arm(ctx context.Context) error {
	var armedAt time.Time
	return database.QuerierFrom(ctx, f.pool).QueryRow(ctx, armStatement).Scan(&armedAt)
}
