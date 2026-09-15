package idempotency

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Retention is how long a stored result is kept after it was written. It is long enough that a
// client may repeat a command it first sent a day ago and short enough that the table does not grow
// with the history of a running demonstration. Cleanup may remove a row only after this period: the
// uniqueness of an invoice or a payment is owned by those records, never by this one.
const Retention = 24 * time.Hour

// ClaimWait bounds how long an attempt waits for the attempt that already holds the same key. A
// command is a short transaction, so a wait this long means the other attempt is stuck rather than
// busy, and the caller is told to come back instead of being held.
const ClaimWait = 2 * time.Second

// The failures of claiming a key. Both are repeatable answers: neither of them changed anything.
var (
	// ErrInProgress reports that another attempt at the same command has not finished within the
	// bounded wait.
	ErrInProgress = errors.New("another attempt at this command is still running")

	// ErrFingerprintMismatch reports that the key already answered a different command. The stored
	// result is left exactly as it was, because it belongs to the command that was actually made.
	ErrFingerprintMismatch = errors.New("the command key already answered another command")
)

// Result is one committed answer: the status and body the first attempt was given, which a repeat
// reproduces. The moment and the request identifier the contract publishes travel inside the body,
// which is what makes a repeat identical without a second copy of either.
type Result struct {
	Status int
	Body   []byte
}

// Claim is a command key as one attempt now holds it: either this attempt must decide the command,
// or the key already holds the answer to give instead.
type Claim struct {
	// Owns reports that this attempt claimed the key and must store what it decides.
	Owns bool

	// Answered is the result an earlier attempt stored, when the key already holds one.
	Answered Result
}

// Store remembers command results. Every method runs on the querier the context carries, so a store
// reached inside a command's transaction writes with that command rather than beside it.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// claimStatement takes the key for this attempt. A key that already exists is left untouched: the
// row that is there belongs to the attempt that wrote it, and its answer is read below.
//
// An attempt that finds the key already taken waits here until the attempt holding it has finished,
// which is what stops two requests with one key from both performing the change. The wait is bounded
// by the statement timeout set around this statement, so a stuck attempt answers busy rather than
// holding the request open.
const claimStatement = `
INSERT INTO idempotency_requests (owner, user_id, command_key, fingerprint)
VALUES ($1, $2, $3, $4)
ON CONFLICT (owner, command_key) DO NOTHING`

// boundClaimStatement bounds the wait of the claim below. A statement timeout covers the whole
// statement including any wait for a competing insert to finish, and it is set through the function
// form because a setting cannot be given a parameter any other way.
const boundClaimStatement = `SELECT set_config('statement_timeout', $1, true)`

// unboundClaimStatement removes that bound again, so the domain work that follows is not cut off by
// a limit that was meant for the wait.
const unboundClaimStatement = `SELECT set_config('statement_timeout', '0', true)`

const storedResultStatement = `
SELECT fingerprint, status_code, body
FROM idempotency_requests
WHERE owner = $1 AND command_key = $2`

// Claim takes the key of a command for this attempt, or reports the answer the key already holds.
//
// It must run inside the transaction that makes the change: the row it writes stays invisible until
// that transaction commits, which is what makes a competing attempt wait for the change rather than
// repeat it, and what leaves no claimed key behind when the change is rolled back.
func (s *Store) Claim(
	ctx context.Context, owner Owner, key Key, fingerprint Fingerprint,
) (Claim, error) {
	querier := database.QuerierFrom(ctx, s.pool)
	if _, err := querier.Exec(ctx, boundClaimStatement, strconv.FormatInt(ClaimWait.Milliseconds(), 10)); err != nil {
		return Claim{}, err
	}

	taken, err := querier.Exec(ctx, claimStatement, owner.name, owner.storedSender(), string(key), string(fingerprint))
	if err != nil && waitTimedOut(err) {
		return Claim{}, ErrInProgress
	}
	if err != nil {
		return Claim{}, err
	}
	// The timeout is lifted before anything else runs: a bounded claim must not bound the command
	// it claimed.
	if _, err = querier.Exec(ctx, unboundClaimStatement); err != nil {
		return Claim{}, err
	}
	if taken.RowsAffected() == 1 {
		return Claim{Owns: true}, nil
	}
	return s.stored(ctx, owner, key, fingerprint)
}

// stored reads the answer the key already holds. The row becomes visible to another transaction only
// once it has committed together with the change, so its result is always there to be read.
func (s *Store) stored(
	ctx context.Context, owner Owner, key Key, fingerprint Fingerprint,
) (Claim, error) {
	var stored Fingerprint
	var answer Result
	err := database.QuerierFrom(ctx, s.pool).QueryRow(ctx, storedResultStatement,
		owner.name, string(key)).Scan(&stored, &answer.Status, &answer.Body)
	if err != nil {
		return Claim{}, err
	}
	if stored != fingerprint {
		return Claim{}, ErrFingerprintMismatch
	}
	return Claim{Answered: answer}, nil
}

// completeStatement stores the answer of the command under the key it was made with. The moment and
// the request identifier of the attempt stay inside the body, and the retention is measured from the
// moment the result was written.
const completeStatement = `
UPDATE idempotency_requests
SET status_code = $3,
    body = $4,
    completed_at = clock_timestamp(),
    retain_until = clock_timestamp() + make_interval(secs => $5)
WHERE owner = $1 AND command_key = $2`

// Complete stores the answer of a command this attempt owns, inside the transaction that made the
// change. A command that decided nothing because it was refused stores the refusal the same way: a
// repeat of a refusal reproduces the refusal rather than deciding the command again.
func (s *Store) Complete(ctx context.Context, owner Owner, key Key, answer Result) error {
	_, err := database.QuerierFrom(ctx, s.pool).Exec(ctx, completeStatement,
		owner.name, string(key), answer.Status, string(answer.Body), Retention.Seconds())
	return err
}

// waitTimedOut reports the failure of a statement the caller had already bounded.
func waitTimedOut(err error) bool {
	var failure *pgconn.PgError
	if !errors.As(err, &failure) {
		return false
	}
	return failure.Code == codeQueryCanceled || failure.Code == codeLockNotAvailable
}

// The SQLSTATE codes this package distinguishes. They are spelled here rather than imported, because
// the driver exposes them for its own use and a reader of this package needs the PostgreSQL meaning.
const (
	codeQueryCanceled    = "57014"
	codeLockNotAvailable = "55P03"
)
