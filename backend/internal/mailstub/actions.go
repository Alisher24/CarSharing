package mailstub

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/demoaction"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ClaimWait bounds how long an attempt waits for the attempt that already holds the same action
// identifier. An action arms one demand, so a wait this long means the other attempt is stuck rather
// than busy, and the caller is told to come back instead of being held.
const ClaimWait = 2 * time.Second

// The failures an action identifier can meet. Both are repeatable answers: neither of them armed a
// second loss.
var (
	// ErrActionConflict reports an identifier that already answered another action. The answer it
	// holds is left exactly as it was, because it belongs to the action that was actually asked for.
	ErrActionConflict = errors.New("the action identifier already answered another action")

	// ErrActionInProgress reports another attempt at the same action that has not finished within the
	// bounded wait.
	ErrActionInProgress = errors.New("another attempt at this action is still running")
)

// Action is what a demonstration asks the stub to do. It is the one action this build declares: the
// loss of the answer to a delivery that was already stored, which is the state a retry is shown from.
type Action string

const (
	// DropNextResponseAfterAccept asks the next delivery to store its letter and lose its answer. The
	// identifier is the one the demonstration's own vocabulary declares, so the terminal that states
	// the action and the stub that applies it cannot come to name it differently.
	DropNextResponseAfterAccept Action = Action(demoaction.DropNextResponseAfterAccept)
)

// Known reports whether an action is one this build declares, which is what a request is judged by
// before anything is armed.
func (a Action) Known() bool { return a == DropNextResponseAfterAccept }

// Result is the answer of one demonstration action: the identifier it was asked under and the moment
// the stub answered it. Both are stored by the attempt that decided the action, so a repeat
// reproduces the first answer rather than stating a second moment.
type Result struct {
	ActionID   string
	ServerTime time.Time
}

// Actions is the memory of the demonstration actions and the demand they arm.
//
// An identifier is claimed, the demand is armed and the answer is stored in one transaction, so an
// action is either remembered with the demand it made or not remembered at all: a repeat of that
// identifier reproduces the stored answer and arms nothing a second time.
type Actions struct {
	pool   *pgxpool.Pool
	faults *Faults
}

// NewActions assembles the demonstration actions over one connection pool.
func NewActions(pool *pgxpool.Pool) (*Actions, error) {
	if pool == nil {
		return nil, errors.New("the demonstration actions must be given a database pool")
	}
	return &Actions{pool: pool, faults: NewFaults(pool)}, nil
}

// claimStatement remembers one action unless its identifier already answered one. The answer is
// written by the statement that claims the identifier, because an action's answer is known the
// moment it is applied: there is no state between claiming one and answering it.
//
// A conflicting row of a committed attempt writes nothing and is read below; a conflicting row of an
// attempt that is still running holds this statement until that attempt finishes, which is what
// stops two requests with one identifier from arming two losses.
const claimStatement = `
INSERT INTO mailstub.demo_actions (action_id, fingerprint, server_time)
VALUES ($1::uuid, $2, clock_timestamp())
ON CONFLICT (action_id) DO NOTHING
RETURNING server_time`

const storedActionStatement = `
SELECT fingerprint, server_time
FROM mailstub.demo_actions
WHERE action_id = $1::uuid`

// Apply arms the demand for the next lost answer under one action identifier, or reproduces the
// answer that identifier already holds. The second answer reports whether the action was already
// applied, which is the header a repeat carries.
//
// The claim, the demand and the stored answer are one transaction, so an action that could not be
// applied leaves neither an armed demand behind nor an identifier that answers nothing.
func (a *Actions) Apply(
	ctx context.Context, actionID string, fingerprint idempotency.Fingerprint,
) (Result, bool, error) {
	var (
		result   Result
		replayed bool
	)
	err := database.InTransaction(ctx, a.pool, func(ctx context.Context) error {
		serverTime, owned, err := a.claim(ctx, actionID, fingerprint)
		if err != nil {
			return err
		}
		result = Result{ActionID: actionID, ServerTime: serverTime}
		if !owned {
			replayed = true
			return nil
		}
		return a.faults.Arm(ctx)
	})
	if err != nil {
		return Result{}, false, err
	}
	return result, replayed, nil
}

// claim takes the identifier for this attempt, or reads the answer the identifier already holds. The
// wait for an attempt that holds the same identifier is bounded by the context, so a stuck attempt
// answers busy rather than holding the request open.
func (a *Actions) claim(
	ctx context.Context, actionID string, fingerprint idempotency.Fingerprint,
) (time.Time, bool, error) {
	attempt, cancel := context.WithTimeout(ctx, ClaimWait)
	defer cancel()

	var claimedAt time.Time
	err := database.QuerierFrom(ctx, a.pool).
		QueryRow(attempt, claimStatement, actionID, string(fingerprint)).Scan(&claimedAt)
	switch {
	case err == nil:
		return claimedAt, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return a.stored(ctx, actionID, fingerprint)
	case attempt.Err() != nil:
		return time.Time{}, false, ErrActionInProgress
	default:
		return time.Time{}, false, err
	}
}

// stored reads the answer the identifier already holds. The row becomes visible to another
// transaction only once it has committed together with the demand it armed, so an answer that is
// there always describes an action that was really applied.
func (a *Actions) stored(
	ctx context.Context, actionID string, fingerprint idempotency.Fingerprint,
) (time.Time, bool, error) {
	var stored idempotency.Fingerprint
	var serverTime time.Time
	err := database.QuerierFrom(ctx, a.pool).
		QueryRow(ctx, storedActionStatement, actionID).Scan(&stored, &serverTime)
	if err != nil {
		return time.Time{}, false, err
	}
	if stored != fingerprint {
		return time.Time{}, false, ErrActionConflict
	}
	return serverTime, false, nil
}
