package invoices

import (
	"github.com/jackc/pgx/v5"
)

func scanInvoice(rows pgx.Row, found *Invoice) error {
	var stored storedInvoiceAmounts
	err := rows.Scan(
		&found.ID,
		&found.RentalID,
		&found.UserID,
		&found.IssuedAt,
		&found.Currency,
		&found.BillingPolicy,
		&found.Completion,
		&stored.exhausted,
		&found.Version,
		&stored.driving.duration,
		&stored.driving.minutes,
		&stored.driving.rate,
		&stored.paused.duration,
		&stored.paused.minutes,
		&stored.paused.rate,
		&stored.total,
		&found.Payment,
		&found.PaymentVersion,
		&found.PaymentUpdatedAt,
		&found.PaidAt,
		&found.FailedAt,
		&found.FailureCode,
	)
	if err != nil {
		return err
	}
	stored.decode(found)
	return nil
}
