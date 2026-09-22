package invoices

import (
	"errors"
	"fmt"
	"math"

	"github.com/Alisher24/CarSharing/backend/internal/billing"
	"github.com/google/uuid"
)

// Validate reports why a draft cannot be written as an invoice, or nil when it can. It guards the
// statement below from a value no row would accept and a reader no row could explain; the storage
// states the same rules again, so a draft that passed here and was refused there is a defect rather
// than the only check there was.
func (d Draft) Validate() error {
	switch {
	case d.RentalID == "":
		return errors.New("an invoice must name the ride it describes")
	case d.UserID == uuid.Nil:
		return errors.New("an invoice must name the account that owes it")
	case d.IssuedAt.IsZero():
		return errors.New("an invoice must state when it was issued")
	case !d.Completion.Known():
		return fmt.Errorf("an invoice cannot state the completion reason %q", d.Completion)
	case d.TotalTyiyn < 0:
		return errors.New("an invoice cannot owe a negative amount")
	}
	total := billing.AmountTyiyn(0)
	for _, line := range []struct {
		name string
		mode billing.Mode
		line Line
	}{
		{"driving", billing.Driving, d.Driving},
		{"paused", billing.Paused, d.Paused},
	} {
		if line.line.Mode != line.mode {
			return fmt.Errorf("the %s line of an invoice describes %q", line.name, line.line.Mode)
		}
		if err := line.line.Validate(); err != nil {
			return err
		}
		if line.line.AmountTyiyn > math.MaxInt64-total {
			return billing.RefusalTotalAmountBeyondRange
		}
		total += line.line.AmountTyiyn
	}
	if total != d.TotalTyiyn {
		return errors.New("the total of an invoice is not the sum of its two lines")
	}
	return nil
}
