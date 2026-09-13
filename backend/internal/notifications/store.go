package notifications

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound reports a notification this account does not hold, whether no such notification exists
// or it belongs to somebody else.
var ErrNotFound = errors.New("no such notification")

// Store is the notification table. Every statement runs on the querier the context carries, so a
// change, the signal announcing it and the domain transition that caused it commit together or not
// at all.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// notificationFields is the shape every read of a notification answers with: the row together with
// the deadline of the rental it is about. That deadline is the rental's rather than the
// notification's, because a warning publishes the deadline it warns about: storing a second copy of
// it would be a value that can disagree with the rental.
const notificationFields = `
    note.id,
    note.user_id,
    note.rental_id,
    note.kind,
    note.created_at,
    note.read_at,
    note.active,
    note.version,
    rental.expires_at`

// notificationColumns reads the notification together with the rental it is about, because a
// notification cannot exist without one: the foreign key and the read agree, so no join here can
// drop a row.
const notificationColumns = `
SELECT` + notificationFields + `
FROM notifications note
JOIN rentals rental ON rental.id = note.rental_id`

// The selections this store reads with. The transitions decide in Go whether a change is a change,
// so the read that precedes one takes the row for update: two of them then wait for each other
// instead of both reading the same version and writing it back.
const (
	notificationOfRentalSelection = notificationColumns + `
WHERE note.rental_id = $1 AND note.kind = $2`

	lockedNotificationOfRentalSelection = notificationOfRentalSelection + `
FOR UPDATE OF note`

	notificationByIDForSelection = notificationColumns + `
WHERE note.id = $1 AND note.user_id = $2`

	lockedNotificationByIDForSelection = notificationByIDForSelection + `
FOR UPDATE OF note`

	// ownerPageSelection walks one owner's collection from a position, newest first. The position is
	// the pair the collection is ordered by, so the page after it continues exactly after the record
	// the cursor was taken from. A page that starts at the newest record states no position: the
	// comparison is then against the last moment there can be, where every stored one is below it,
	// and the identifier it is compared with second never has to decide anything.
	ownerPageSelection = notificationColumns + `
WHERE note.user_id = $1
  AND (note.created_at, note.id) <
      (COALESCE($2::timestamptz, 'infinity'::timestamptz),
       COALESCE($3::uuid, '00000000-0000-0000-0000-000000000000'::uuid))
ORDER BY note.created_at DESC, note.id DESC
LIMIT $4`
)

// Create stores the notification of one rental and kind at the moment the transaction fixed, and
// reports whether this call stored it. A notification that already exists for that rental and kind
// is answered as it stands: the unique key makes the repeated attempt write nothing, so the moment
// the first one stored is never replaced and no signal is recorded for a change that did not happen.
func (s *Store) Create(
	ctx context.Context, about About, at time.Time,
) (Notification, bool, error) {
	created, err := Created(about, at)
	if err != nil {
		return Notification{}, false, err
	}
	written, err := s.insert(ctx, created)
	if err != nil {
		return Notification{}, false, err
	}
	if !written {
		existing, err := readNotification(ctx, s.pool,
			notificationOfRentalSelection, about.RentalID, about.Kind)
		return existing, false, err
	}
	// The record is read back through the selection every reader of this store uses, so the write is
	// the only statement that has to know the shape of a row: what a creation answers is what a read
	// of the same notification answers.
	stored, err := readNotification(ctx, s.pool, notificationByIDForSelection, created.ID, created.UserID)
	if err != nil {
		return Notification{}, false, err
	}
	announced, err := s.announce(ctx, stored)
	if err != nil {
		return Notification{}, false, err
	}
	return announced, true, nil
}

// Deactivate makes the notification of one rental and kind inactive and moves its version, and
// reports the stored notification together with whether this call changed it. A rental that never
// had a notification leaves the table as it is — the answer is then an empty notification and false,
// because there is none to report — and so does one whose notification is already inactive: a
// warning that was never created does not appear after the fact.
func (s *Store) Deactivate(ctx context.Context, rentalID string, kind Kind) (Notification, bool, error) {
	stored, err := readNotification(ctx, s.pool, lockedNotificationOfRentalSelection, rentalID, kind)
	if errors.Is(err, ErrNotFound) {
		return Notification{}, false, nil
	}
	if err != nil {
		return Notification{}, false, err
	}
	next, changed := stored.Deactivated()
	if !changed {
		return stored, false, nil
	}
	return s.change(ctx, next)
}

// MarkRead stores the moment one notification of this account was read and moves its version, and
// reports the stored notification together with whether this call changed it. The notification stays
// active: having acknowledged a warning is a different fact from the reservation having ended. A
// repeated read answers the representation the first one stored, and a notification of another
// account is reported as absent.
func (s *Store) MarkRead(
	ctx context.Context, owner uuid.UUID, id string, at time.Time,
) (Notification, bool, error) {
	stored, err := readNotification(ctx, s.pool, lockedNotificationByIDForSelection, id, owner)
	if err != nil {
		return Notification{}, false, err
	}
	next, changed := stored.Read(at)
	if !changed {
		return stored, false, nil
	}
	return s.change(ctx, next)
}

// ByID reads one notification this account owns. Another account's notification is reported as
// absent rather than as forbidden, so the answer cannot be used to learn that it exists.
func (s *Store) ByID(ctx context.Context, owner uuid.UUID, id string) (Notification, error) {
	return readNotification(ctx, s.pool, notificationByIDForSelection, id, owner)
}

// PageSize is how many notifications one page of the collection carries when the client states no
// limit. It is the contract's declared default, stated here so the package that pages the collection
// and the operation that serves it cannot disagree about it.
const PageSize = 20

// Position is where a page of a collection starts: the sort key of the notification the previous
// page ended with. It is the pair the collection is ordered by, so a page read after it continues
// exactly after that notification rather than at one the client guessed.
type Position struct {
	CreatedAt time.Time
	ID        string
}

// Page is one page of an owner's notifications: the records it holds, and the position the page
// after it starts from. The position is absent on the last and on the empty page, which is the
// `next_cursor: null` the contract declares.
type Page struct {
	Notifications []Notification
	Next          *Position
}

// ReadPage reads one page of an owner's notifications after a position, in the order the collection
// publishes them: newest first, with the identifier as the tie-break. A page is read with one record
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

	records := make([]Notification, 0, limit+1)
	for rows.Next() {
		var found Notification
		if err = scanNotification(rows, &found); err != nil {
			return Page{}, err
		}
		records = append(records, found)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	if len(records) <= limit {
		return Page{Notifications: records}, nil
	}

	records = records[:limit]
	last := records[len(records)-1]
	return Page{
		Notifications: records,
		Next:          &Position{CreatedAt: last.CreatedAt, ID: last.ID},
	}, nil
}

// positionMoment and positionIdentifier read the two parts of an optional position. A page that
// starts at the newest record has none, and each part of the search key becomes a null the
// selection reads as "before everything".
func positionMoment(after *Position) *time.Time {
	if after == nil {
		return nil
	}
	moment := after.CreatedAt
	return &moment
}

func positionIdentifier(after *Position) *string {
	if after == nil {
		return nil
	}
	identifier := after.ID
	return &identifier
}

// insert writes one notification unless the rental already has one of its kind. The conflict is the
// rule rather than a failure, so the statement reports whether it wrote; the caller reads the record
// back through the selection every reader of this store uses rather than through a second shape.
const insertNotificationStatement = `
INSERT INTO notifications (id, user_id, rental_id, kind, created_at, read_at, active, version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (rental_id, kind) DO NOTHING
RETURNING id`

func (s *Store) insert(ctx context.Context, about Notification) (bool, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, insertNotificationStatement,
		about.ID, about.UserID, about.RentalID, about.Kind, about.CreatedAt, about.ReadAt,
		about.Active, about.Version)
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

const updateNotificationStatement = `
UPDATE notifications
SET read_at = $2, active = $3, version = $4
WHERE id = $1
RETURNING id`

func (s *Store) update(ctx context.Context, changed Notification) error {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, updateNotificationStatement,
		changed.ID, changed.ReadAt, changed.Active, changed.Version)
	if err != nil {
		return err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return err
		}
		return ErrNotFound
	}
	return rows.Err()
}

// change writes a representation that replaced a stored one, reads it back and reports it as
// changed.
func (s *Store) change(ctx context.Context, next Notification) (Notification, bool, error) {
	if err := s.update(ctx, next); err != nil {
		return Notification{}, false, err
	}
	stored, err := readNotification(ctx, s.pool, notificationByIDForSelection, next.ID, next.UserID)
	if err != nil {
		return Notification{}, false, err
	}
	announced, err := s.announce(ctx, stored)
	if err != nil {
		return Notification{}, false, err
	}
	return announced, true, nil
}

// announce records the personal signal of one stored change. It is written through the querier the
// context carries, so it commits with the change it describes: a stored change nobody is told about
// and a signal about a change that was not stored are both states this store cannot reach.
func (s *Store) announce(ctx context.Context, stored Notification) (Notification, error) {
	err := events.Record(ctx, s.pool, events.Signal{
		Kind:       events.NotificationChanged,
		ResourceID: stored.ID,
		Version:    stored.Version,
		Recipient:  stored.UserID,
	})
	if err != nil {
		return Notification{}, err
	}
	return stored, nil
}

// readNotification reads at most one notification, so that a selection matching several rows is
// reported as a failure of the caller's expectation rather than silently answering the first.
func readNotification(
	ctx context.Context, pool *pgxpool.Pool, selection string, arguments ...any,
) (Notification, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, selection, arguments...)
	if err != nil {
		return Notification{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Notification{}, err
		}
		return Notification{}, ErrNotFound
	}
	var found Notification
	if err = scanNotification(rows, &found); err != nil {
		return Notification{}, err
	}
	if rows.Next() {
		return Notification{}, errors.New("the selection matched more than one notification")
	}
	return found, rows.Err()
}

func scanNotification(rows pgx.Rows, found *Notification) error {
	return rows.Scan(
		&found.ID,
		&found.UserID,
		&found.RentalID,
		&found.Kind,
		&found.CreatedAt,
		&found.ReadAt,
		&found.Active,
		&found.Version,
		&found.ExpiresAt,
	)
}
