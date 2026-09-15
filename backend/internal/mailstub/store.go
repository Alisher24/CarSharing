package mailstub

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The failures a store reports about a letter rather than about the database.
var (
	// ErrDeliveryConflict reports a delivery key that already holds a different letter. The stored
	// letter is left exactly as it was: it is the evidence of what was delivered once, so a second
	// delivery under the same key cannot replace it.
	ErrDeliveryConflict = errors.New("the delivery key already holds a different letter")

	// ErrMessageNotFound reports a letter this box does not hold, whether no such letter exists or
	// its identifier is not one this build wrote.
	ErrMessageNotFound = errors.New("no such message")
)

// The page sizes the collection declares: the size a request that states none is read with, and the
// largest one a request may ask for.
const (
	PageSize    = 20
	MaxPageSize = 100
)

// Store is the mail box. Every statement names the mail schema rather than relying on the search
// path, because the box belongs to a schema of its own and the process that reads it is connected as
// a role that reaches nothing else.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// messageFields is the shape every read of a letter answers with. One declaration keeps the read of
// one letter and the read of a page from drifting apart.
const messageFields = `
    message.id,
    message.delivery_key,
    message.recipient,
    message.subject,
    message.body,
    message.accepted_at`

const (
	messageByIDSelection = `
SELECT` + messageFields + `
FROM mailstub.messages message
WHERE message.id = $1`

	messageByKeySelection = `
SELECT` + messageFields + `
FROM mailstub.messages message
WHERE message.delivery_key = $1`

	// pageSelection walks the box from a position, newest first. The position is the pair the
	// collection is ordered by, so the page after it continues exactly after the letter the cursor
	// was taken from. A page that starts at the newest letter states no position: the comparison is
	// then against the last moment there can be, where every stored one is below it, and the
	// identifier it is compared with second never has to decide anything.
	pageSelection = `
SELECT` + messageFields + `
FROM mailstub.messages message
WHERE (message.accepted_at, message.id) <
      (COALESCE($1::timestamptz, 'infinity'::timestamptz),
       COALESCE($2::uuid, '00000000-0000-0000-0000-000000000000'::uuid))
ORDER BY message.accepted_at DESC, message.id DESC
LIMIT $3`
)

// Accept stores one letter under its delivery key, and reports whether this call stored it or
// answered a letter the key already held.
//
// A repeat answers the stored letter whole — its identifier, its content and the moment it was first
// accepted — rather than the request that repeated it, because those three are what the delivery
// receipt states and what the box shows. The same key with anything else to say is a conflict rather
// than a repeat.
//
// The insert is what decides, not a read before it: two deliveries of one key racing each other are
// resolved by the unique constraint, so exactly one letter is stored whatever the two processes saw
// before they wrote.
func (s *Store) Accept(
	ctx context.Context, key Key, request Request, acceptedAt time.Time,
) (Message, bool, error) {
	if !key.Known() {
		return Message{}, false, ErrInvalidDeliveryKey
	}
	if err := request.Validate(); err != nil {
		return Message{}, false, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Message{}, false, err
	}
	written, err := s.insert(ctx, id.String(), key, request, acceptedAt)
	if err != nil {
		return Message{}, false, err
	}
	if !written {
		stored, err := s.read(ctx, messageByKeySelection, key.String())
		if err != nil {
			return Message{}, false, err
		}
		if !stored.states(request) {
			return Message{}, false, fmt.Errorf("%w: %s", ErrDeliveryConflict, key)
		}
		return stored, false, nil
	}
	// The letter is read back through the selection every reader of this store uses, so the write is
	// the only statement that has to know the shape of a row: what a delivery is answered with is
	// what a later read of the same letter answers.
	stored, err := s.read(ctx, messageByIDSelection, id.String())
	if err != nil {
		return Message{}, false, err
	}
	return stored, true, nil
}

// insertStatement writes one letter unless its key already holds one. The conflict is a repeat rather
// than a failure, so the statement reports whether it wrote: the key already answered a delivery, and
// what that delivery stored is read back by the caller.
//
// Every value is cast to the type of its column, so a moment or an identifier reaches the row as
// the value it is rather than as the text a driver might have inferred a type for.
const insertStatement = `
INSERT INTO mailstub.messages (id, delivery_key, recipient, subject, body, accepted_at)
VALUES ($1::uuid, $2, $3, $4, $5, $6::timestamptz)
ON CONFLICT (delivery_key) DO NOTHING
RETURNING id`

func (s *Store) insert(
	ctx context.Context, id string, key Key, request Request, acceptedAt time.Time,
) (bool, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, insertStatement,
		id, key.String(), request.To, request.Subject, request.Text, acceptedAt)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	if !rows.Next() {
		return false, rows.Err()
	}
	var written string
	if err = rows.Scan(&written); err != nil {
		return false, err
	}
	return true, rows.Err()
}

// ByID reads one letter of the box. A letter that is not there is reported as absent rather than
// answered with an empty one.
func (s *Store) ByID(ctx context.Context, id string) (Message, error) {
	return s.read(ctx, messageByIDSelection, id)
}

// Position is where a page of the box starts: the sort key of the letter the previous page ended
// with. It is the pair the collection is ordered by, so the page after it continues exactly after
// that letter rather than at one the client guessed.
type Position struct {
	AcceptedAt time.Time
	ID         string
}

// Page is one page of the box: the letters it holds, and the position the page after it starts from.
// The position is absent on the last and on the empty page, which is the `next_cursor: null` the
// contract declares.
type Page struct {
	Messages []Message
	Next     *Position
}

// ReadPage reads one page of the box after a position, in the order the collection publishes: newest
// first, with the identifier deciding two letters accepted at the same moment. A page is read with
// one letter more than it publishes, so whether anything follows is decided by the database rather
// than by the page being shorter than the limit.
func (s *Store) ReadPage(ctx context.Context, after *Position, limit int) (Page, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, pageSelection,
		positionMoment(after), positionIdentifier(after), limit+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()

	messages := make([]Message, 0, limit+1)
	for rows.Next() {
		var found Message
		if err = scanMessage(rows, &found); err != nil {
			return Page{}, err
		}
		messages = append(messages, found)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	if len(messages) <= limit {
		return Page{Messages: messages}, nil
	}

	messages = messages[:limit]
	last := messages[len(messages)-1]
	return Page{Messages: messages, Next: &Position{AcceptedAt: last.AcceptedAt, ID: last.ID}}, nil
}

// positionMoment and positionIdentifier read the two parts of an optional position. A page that
// starts at the newest letter has none, and each part of the search key becomes a null the selection
// reads as "before everything".
func positionMoment(after *Position) *time.Time {
	if after == nil {
		return nil
	}
	moment := after.AcceptedAt
	return &moment
}

func positionIdentifier(after *Position) *string {
	if after == nil {
		return nil
	}
	identifier := after.ID
	return &identifier
}

// read reads at most one letter, so that a selection matching several rows is reported as a failure
// of the caller's expectation rather than silently answering the first.
func (s *Store) read(ctx context.Context, selection string, arguments ...any) (Message, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, selection, arguments...)
	if err != nil {
		return Message{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Message{}, err
		}
		return Message{}, ErrMessageNotFound
	}
	var found Message
	if err = scanMessage(rows, &found); err != nil {
		return Message{}, err
	}
	if rows.Next() {
		return Message{}, errors.New("the selection matched more than one message")
	}
	return found, rows.Err()
}

func scanMessage(rows pgx.Rows, found *Message) error {
	return rows.Scan(
		&found.ID,
		&found.DeliveryKey,
		&found.To,
		&found.Subject,
		&found.Text,
		&found.AcceptedAt,
	)
}
