package invoices

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
)

// Page is one page of an account's invoices: the invoices it holds, each with the state of its
// payment, and the position the page after it starts from. The position is absent on the last and on
// the empty page, which is the `next_cursor: null` the contract declares.
type Page struct {
	Invoices []Invoice
	Next     *cursor.Position
}

// ownerPageSelection walks one account's invoices from a position, newest first. It reads the payment
// beside the invoice through the same columns a read of one invoice uses, because the contract
// publishes the two together.
//
// A page that starts at the newest invoice states no position: the comparison is then against the
// last moment there can be, where every stored one is below it, and the identifier it is compared
// with second never has to decide anything.
var ownerPageSelection = invoiceColumns + `
WHERE invoice.user_id = $1
  AND ` + cursor.Descending("invoice.issued_at", "invoice.id", 2)

// ReadPage reads one page of an account's invoices after a position, in the order the collection
// publishes them: newest first, with the identifier as the tie-break. A page is read with one invoice
// more than it publishes, so whether anything follows is decided by the database rather than by the
// page being shorter than the limit.
func (s *Store) ReadPage(
	ctx context.Context, owner uuid.UUID, after *cursor.Position, limit int,
) (Page, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, ownerPageSelection,
		owner, after.MomentArgument(), after.IdentifierArgument(), limit+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()

	found := make([]Invoice, 0, limit+1)
	for rows.Next() {
		var issued Invoice
		if err = scanInvoice(rows, &issued); err != nil {
			return Page{}, err
		}
		found = append(found, issued)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	return pageOf(found, limit), nil
}

// pageOf cuts the invoices read into the page that is published and the position the page after it
// starts from.
func pageOf(found []Invoice, limit int) Page {
	published, next := cursor.Cut(found, limit, invoicePosition)
	return Page{Invoices: published, Next: next}
}

func invoicePosition(item Invoice) cursor.Position {
	return cursor.Position{Moment: item.IssuedAt, ID: item.ID}
}
