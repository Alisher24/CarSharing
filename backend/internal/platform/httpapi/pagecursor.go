package httpapi

import (
	"strconv"

	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/google/uuid"
)

// limitParameter is the query parameter every paginated collection reads its page size from, and
// therefore the parameter every one of their cursors is bound to. It is declared once because a
// cursor is refused when the parameters it was issued under are spelled differently from the ones it
// is presented with.
const limitParameter = "limit"

// pageLimitOf is how many records one page carries: what the request asked for, or the size the
// collection declares when the request asks for nothing. The contract states the same default, and a
// client that omits the parameter continues with the page size its first page used.
func pageLimitOf(requested *int, declared int) int {
	if requested == nil {
		return declared
	}
	return *requested
}

// accountPageScope names the collection, the account and the parameters a cursor of one of an
// account's own collections is bound to. The operation is the identifier the contract gives it, so a
// cursor issued for another collection, another account or another page size is refused by its
// signature rather than by a comparison somebody has to remember to write.
func accountPageScope(operation string, owner uuid.UUID, limit int) cursor.Scope {
	return cursor.OperationOn(operation, owner,
		cursor.Parameter{Name: limitParameter, Value: strconv.Itoa(limit)})
}

// pagePositionOf reads the position a presented cursor names, or nil when the request carries none.
// Every failure — an unreadable token, a signature that does not match its payload, a cursor issued
// for another operation, account or parameter set — is reported alike, because each of them is the
// one refusal the contract declares for a cursor and a client may not tell them apart.
func pagePositionOf(signer *cursor.Signer, presented *string, scope cursor.Scope) (*cursor.Position, error) {
	if presented == nil {
		return nil, nil
	}
	position, err := signer.Read(*presented, scope)
	if err != nil {
		return nil, err
	}
	return &position, nil
}
