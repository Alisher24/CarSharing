package invoices

import (
	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

type storedInvoiceAmounts struct {
	driving   storedLine
	paused    storedLine
	total     int64
	exhausted []string
}

func (stored storedInvoiceAmounts) decode(found *Invoice) {
	found.Exhausted = fleet.SourceKinds(stored.exhausted)
	found.Driving = stored.driving.decoded(billing.Driving)
	found.Paused = stored.paused.decoded(billing.Paused)
	found.TotalTyiyn = billing.AmountTyiyn(stored.total)
}
