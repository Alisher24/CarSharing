package outbox

import (
	"context"
	"errors"
	"testing"
)

// A task this build cannot deliver must be kept rather than reported as delivered: nothing was
// attempted, so nothing can be recorded as a success.
func TestDeliveriesRefuseAKindThisBuildDoesNotDeliver(t *testing.T) {
	delivered := false
	deliveries := Deliveries{"known.kind": func(context.Context, Task) error {
		delivered = true
		return nil
	}}

	err := deliveries.Deliver(context.Background(), Task{Kind: "unknown.kind"})
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("unknown kind answered %v", err)
	}
	if delivered {
		t.Fatal("an unknown kind was delivered")
	}

	if err = deliveries.Deliver(context.Background(), Task{Kind: "known.kind"}); err != nil {
		t.Fatalf("a declared kind answered %v", err)
	}
	if !delivered {
		t.Fatal("a declared kind was not delivered")
	}
}
