package outbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Task is one durable piece of work. Its fields are the change it announces rather than a free-form
// payload: what kind of change it is, which resource it happened to, the version that resource
// reached, and the account it belongs to.
type Task struct {
	// ID is the identifier storage gave this task.
	ID uuid.UUID

	// Kind names what must be delivered. It is unrestricted text: a task of a kind this build does
	// not know stays in the queue with its error instead of being refused or reported as delivered.
	Kind string

	// ResourceID and Version describe the change the task announces.
	ResourceID string
	Version    int64

	// Recipient is the account the change belongs to, empty when it may be published to everyone.
	Recipient uuid.UUID

	// Attempts is how many claims this task has had, including the one that produced it here.
	Attempts int
}

// Deliver hands one claimed task to whatever delivers it. An error means the task was not delivered:
// the worker records the failure and keeps the task.
type Deliver func(context.Context, Task) error

// ErrUnknownKind reports a task whose kind this build has no delivery for. It is not a delivery
// failure: nothing was attempted and the task must stay visible.
var ErrUnknownKind = errors.New("no delivery is declared for this task kind")

// Deliveries pairs each kind this build can deliver with the behaviour that delivers it, so a new
// kind is one entry where the deliveries are assembled rather than a branch inside the worker.
type Deliveries map[string]Deliver

// Deliver runs the delivery of one task, or reports a kind this build does not deliver.
func (d Deliveries) Deliver(ctx context.Context, task Task) error {
	deliver, known := d[task.Kind]
	if !known {
		return fmt.Errorf("%w: %q", ErrUnknownKind, task.Kind)
	}
	return deliver(ctx, task)
}

// Claim is one attempt's right to settle a task: the task it took and the token this attempt was
// given. Only the holder of the current token, before its lease runs out, may confirm the task or
// record its failure.
type Claim struct {
	Task  Task
	Token uuid.UUID
}
