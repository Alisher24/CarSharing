package rentals

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// WarningOperations are the two notification-owned changes caused by a reservation transition.
// Their implementation is supplied by the composition root and runs in the caller's transaction.
type WarningOperations struct {
	Create func(context.Context, uuid.UUID, string, time.Time) (bool, error)
	End    func(context.Context, string) error
}

func (o WarningOperations) complete() bool {
	return o.Create != nil && o.End != nil
}
