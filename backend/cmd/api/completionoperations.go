package main

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/google/uuid"
)

type completionOperations struct {
	completer *notifications.Completer
}

func completionRecorder(completer *notifications.Completer) rentals.RecordCompletion {
	return completionOperations{completer: completer}.record
}

func (o completionOperations) record(
	ctx context.Context,
	owner uuid.UUID,
	rentalID string,
	report rentals.CompletionReport,
	at time.Time,
) error {
	return o.completer.Record(ctx, owner, rentalID, notifications.Completion{
		InvoiceID: report.InvoiceID,
		Reason:    report.Reason,
		EndedAt:   report.EndedAt,
		Exhausted: report.Exhausted,
	}, at)
}
