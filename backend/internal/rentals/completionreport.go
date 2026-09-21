package rentals

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/google/uuid"
)

// CompletionReport is what a finished ride asks the notification owner to report.
type CompletionReport struct {
	InvoiceID string
	Reason    completion.Reason
	EndedAt   time.Time
	Exhausted []fleet.SourceKind
}

// RecordCompletion stores the report caused by one finished ride.
type RecordCompletion func(
	context.Context,
	uuid.UUID,
	string,
	CompletionReport,
	time.Time,
) error
