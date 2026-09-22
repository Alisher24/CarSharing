package invoices

import (
	"errors"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
)

// Validate reports why a line cannot be one of an invoice, or nil when it can.
func (l Line) Validate() error {
	switch {
	case l.Mode != billing.Driving && l.Mode != billing.Paused:
		return errors.New("a line must describe one of the two modes")
	case l.Duration < 0:
		return billing.RefusalNegativeDuration
	case l.Minutes < 0:
		return billing.RefusalNegativeMinutes
	case l.Rate < 0:
		return billing.RefusalNegativeRate
	case l.AmountTyiyn < 0:
		return errors.New("an amount cannot be negative")
	}
	priced, err := billing.PricedAt(l.Rate, l.Minutes)
	if err != nil {
		return err
	}
	if priced != l.AmountTyiyn {
		return errors.New("an amount is not the rate of its line times the minutes begun in it")
	}
	return nil
}
