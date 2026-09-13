package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the queue in PostgreSQL. Every statement runs on the querier the context carries, so a
// task recorded inside a transaction commits with it and cannot outlive a rollback.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// storedTask is one task as the insert reads it. A public change is written as a null recipient
// rather than as an all-zero identifier, so the column means what it says.
type storedTask struct {
	Kind       string     `json:"kind"`
	ResourceID string     `json:"resource_id"`
	Version    int64      `json:"version"`
	Recipient  *uuid.UUID `json:"recipient_id"`
}

// Record writes the given tasks in one statement. It is meant to be called inside the transaction
// that makes the change they announce, which is what ties the two together in both directions.
const recordStatement = `
INSERT INTO outbox (kind, resource_id, version, recipient_id)
SELECT kind, resource_id, version, recipient_id
FROM jsonb_to_recordset($1::jsonb)
    AS task(kind text, resource_id uuid, version bigint, recipient_id uuid)`

func (s *Store) Record(ctx context.Context, tasks ...Task) error {
	if len(tasks) == 0 {
		return nil
	}
	stored := make([]storedTask, 0, len(tasks))
	for _, task := range tasks {
		recipient := task.Recipient
		if recipient == uuid.Nil {
			stored = append(stored, storedTask{task.Kind, task.ResourceID, task.Version, nil})
			continue
		}
		stored = append(stored, storedTask{task.Kind, task.ResourceID, task.Version, &recipient})
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	_, err = database.QuerierFrom(ctx, s.pool).Exec(ctx, recordStatement, string(payload))
	return err
}

// Claim takes the task that has waited longest and is not held by a live lease, gives it to this
// attempt under a fresh token, and counts the attempt. It is one statement, so the row lock it takes
// is released before Claim returns: nothing is locked while the task is being delivered.
//
// A task whose lease has run out is claimable again, which is what recovers the work of a worker
// that died holding it.
const claimStatement = `
UPDATE outbox
SET lease_token = $1,
    lease_expires_at = now() + make_interval(secs => $2),
    attempts = attempts + 1
WHERE id = (
    SELECT id
    FROM outbox
    WHERE completed_at IS NULL
      AND next_attempt_at <= now()
      AND (lease_expires_at IS NULL OR lease_expires_at <= now())
    ORDER BY next_attempt_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, kind, resource_id, version, recipient_id, attempts`

func (s *Store) Claim(ctx context.Context, lease time.Duration) (Claim, bool, error) {
	token := uuid.New()
	var task Task
	var recipient *uuid.UUID
	err := database.QuerierFrom(ctx, s.pool).QueryRow(ctx, claimStatement, token, lease.Seconds()).Scan(
		&task.ID, &task.Kind, &task.ResourceID, &task.Version, &recipient, &task.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return Claim{}, false, nil
	}
	if err != nil {
		return Claim{}, false, err
	}
	if recipient != nil {
		task.Recipient = *recipient
	}
	return Claim{Task: task, Token: token}, true, nil
}

// The two ways an attempt settles its task. Both name the token and require a lease that has not run
// out: a worker that comes back after its lease expired does not confirm or rewrite the attempt of
// whoever holds the task now. Both report the number of rows they changed, which is how the worker
// learns that the outcome belongs to someone else.
const (
	completeStatement = `
UPDATE outbox
SET completed_at = now(), lease_token = NULL, lease_expires_at = NULL, last_error = NULL
WHERE id = $1 AND lease_token = $2 AND lease_expires_at > now()`

	failStatement = `
UPDATE outbox
SET lease_token = NULL,
    lease_expires_at = NULL,
    last_error = $3,
    next_attempt_at = now() + make_interval(secs => $4)
WHERE id = $1 AND lease_token = $2 AND lease_expires_at > now()`
)

// Complete records that the task was delivered. It reports whether this attempt still owned the
// task: false means the lease had run out and the outcome is not this attempt's to report.
func (s *Store) Complete(ctx context.Context, claim Claim) (bool, error) {
	tag, err := database.QuerierFrom(ctx, s.pool).Exec(ctx, completeStatement, claim.Task.ID, claim.Token)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// Fail records why the delivery failed and when the task may be claimed again. The task itself
// stays: a failure is not a deletion, and it is never turned into a success.
func (s *Store) Fail(ctx context.Context, claim Claim, failure string, after time.Duration) (bool, error) {
	tag, err := database.QuerierFrom(ctx, s.pool).Exec(ctx, failStatement,
		claim.Task.ID, claim.Token, failure, after.Seconds())
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// Delivered tasks of the named kinds are deleted in small batches once they are older than the
// retention. A task that has not been delivered is never deleted, and neither is a kind the caller
// does not name: the retention policy of the signal queue is not a policy for every future purpose.
const deleteCompletedStatement = `
DELETE FROM outbox
WHERE id IN (
    SELECT id
    FROM outbox
    WHERE kind = ANY($1)
      AND completed_at IS NOT NULL
      AND completed_at <= now() - make_interval(secs => $2)
    ORDER BY completed_at
    LIMIT $3
)`

func (s *Store) DeleteCompleted(
	ctx context.Context, kinds []string, retention time.Duration, limit int,
) (int64, error) {
	tag, err := database.QuerierFrom(ctx, s.pool).Exec(ctx, deleteCompletedStatement,
		kinds, retention.Seconds(), limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Pending describes what the queue still owes: how many tasks are undelivered and how long the
// oldest of them has been waiting, measured by the database clock.
type Pending struct {
	Count     int64
	OldestAge time.Duration
}

const pendingStatement = `
SELECT count(*), coalesce(extract(epoch FROM now() - min(created_at)), 0)::float8
FROM outbox
WHERE completed_at IS NULL`

func (s *Store) Pending(ctx context.Context) (Pending, error) {
	var pending Pending
	var oldestSeconds float64
	err := database.QuerierFrom(ctx, s.pool).QueryRow(ctx, pendingStatement).Scan(
		&pending.Count, &oldestSeconds)
	if err != nil {
		return Pending{}, err
	}
	pending.OldestAge = time.Duration(oldestSeconds * float64(time.Second))
	return pending, nil
}
