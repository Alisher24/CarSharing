package invoices

import (
	"context"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
)

// PageSize is how many invoices one page of the collection carries when the client states no limit.
// It is the contract's declared default, stated here so the package that pages the collection and the
// operation that serves it cannot disagree about it.
const PageSize = 20

// Position is where a page of the collection starts: the sort key of the invoice the previous page
// ended with. It is the pair the collection is ordered by, so a page read after it continues exactly
// after that invoice rather than at one the client guessed.
type Position struct {
	IssuedAt time.Time
	ID       string
}

// Page is one page of an account's invoices: the invoices it holds, each with the state of its
// payment, and the position the page after it starts from. The position is absent on the last and on
// the empty page, which is the `next_cursor: null` the contract declares.
type Page struct {
	Invoices []Invoice
	Next     *Position
}

// ownerPageSelection walks one account's invoices from a position, newest first. It reads the payment
// beside the invoice through the same columns a read of one invoice uses, because the contract
// publishes the two together.
//
// A page that starts at the newest invoice states no position: the comparison is then against the
// last moment there can be, where every stored one is below it, and the identifier it is compared
// with second never has to decide anything.
const ownerPageSelection = invoiceColumns + `
WHERE invoice.user_id = $1
  AND (invoice.issued_at, invoice.id) <
      (COALESCE($2::timestamptz, 'infinity'::timestamptz),
       COALESCE($3::uuid, '00000000-0000-0000-0000-000000000000'::uuid))
ORDER BY invoice.issued_at DESC, invoice.id DESC
LIMIT $4`

// ReadPage reads one page of an account's invoices after a position, in the order the collection
// publishes them: newest first, with the identifier as the tie-break. A page is read with one invoice
// more than it publishes, so whether anything follows is decided by the database rather than by the
// page being shorter than the limit.
func (s *Store) ReadPage(
	ctx context.Context, owner uuid.UUID, after *Position, limit int,
) (Page, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, ownerPageSelection,
		owner, positionMoment(after), positionIdentifier(after), limit+1)
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
	if len(found) <= limit {
		return Page{Invoices: found}
	}
	published := found[:limit]
	last := published[len(published)-1]
	return Page{
		Invoices: published,
		Next:     &Position{IssuedAt: last.IssuedAt, ID: last.ID},
	}
}

// positionMoment and positionIdentifier read the two parts of an optional position. A page that
// starts at the newest invoice has none, and each part of the search key becomes a null the selection
// reads as "before everything".
func positionMoment(after *Position) *time.Time {
	if after == nil {
		return nil
	}
	moment := after.IssuedAt
	return &moment
}

func positionIdentifier(after *Position) *string {
	if after == nil {
		return nil
	}
	identifier := after.ID
	return &identifier
}
