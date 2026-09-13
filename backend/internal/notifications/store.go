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

// notificationFields is the shape every read and every write of a notification returns. One
// declaration keeps a row read by identifier, by rental or by owner from drifting apart.
const notificationFields = `
    id,
    user_id,
    rental_id,
    kind,
    created_at,
    read_at,
    active,
    version`

const notificationColumns = `
SELECT` + notificationFields + `
FROM notifications`

// The selections this store reads with. The transitions decide in Go whether a change is a change,
// so the read that precedes one takes the row for update: two of them then wait for each other
// instead of both reading the same version and writing it back.
const (
	notificationOfRentalSelection = notificationColumns + `
WHERE rental_id = $1 AND kind = $2`

	lockedNotificationOfRentalSelection = notificationOfRentalSelection + `
FOR UPDATE`

	notificationByIDForSelection = notificationColumns + `
WHERE id = $1 AND user_id = $2`

	lockedNotificationByIDForSelection = notificationByIDForSelection + `
FOR UPDATE`

	newestOfOwnerSelection = notificationColumns + `
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2`
)

// Create stores the notification of one rental and kind at the moment the transaction fixed, and
// reports whether this call stored it. A notification that already exists for that rental and kind
// is answered as it stands: the unique key makes the repeated attempt write nothing, so the moment
// the first one stored is never replaced and no signal is recorded for a change that did not happen.
func (s *Store) Create(ctx context.Context, about About, at time.Time) (Notification, bool, error) {
	created, err := Created(about, at)
	if err != nil {
		return Notification{}, false, err
	}
	stored, written, err := s.insert(ctx, created)
	if err != nil {
		return Notification{}, false, err
	}
	if !written {
		existing, err := readNotification(ctx, s.pool,
			notificationOfRentalSelection, about.RentalID, about.Kind)
		return existing, false, err
	}
	announced, err := s.announce(ctx, stored)
	if err != nil {
		return Notification{}, false, err
	}
	return announced, true, nil
}

// Deactivate makes the notification of one rental and kind inactive and moves its version, and
// reports the stored notification together with whether this call changed it. A rental that never
// had a notification leaves the table as it is, and so does one whose notification is already
// inactive: a warning that was never created does not appear after the fact.
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

// Newest reads the newest notifications of one account, in the order the collection publishes them:
// newest first, with the identifier as the tie-break. Only this account's records are read, so
// another account's notification neither appears in the answer nor takes a place in it.
func (s *Store) Newest(ctx context.Context, owner uuid.UUID, limit int) ([]Notification, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, newestOfOwnerSelection, owner, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	newest := make([]Notification, 0, limit)
	for rows.Next() {
		var found Notification
		if err = scanNotification(rows, &found); err != nil {
			return nil, err
		}
		newest = append(newest, found)
	}
	return newest, rows.Err()
}

// insert writes one notification unless the rental already has one of its kind. The conflict is the
// rule rather than a failure, so the statement reports whether it wrote and the caller answers the
// stored record.
const insertNotificationStatement = `
INSERT INTO notifications (id, user_id, rental_id, kind, created_at, read_at, active, version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (rental_id, kind) DO NOTHING
RETURNING` + notificationFields

func (s *Store) insert(
	ctx context.Context, about Notification,
) (Notification, bool, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, insertNotificationStatement,
		about.ID, about.UserID, about.RentalID, about.Kind, about.CreatedAt, about.ReadAt,
		about.Active, about.Version)
	if err != nil {
		return Notification{}, false, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Notification{}, false, err
		}
		return Notification{}, false, nil
	}
	var stored Notification
	if err = scanNotification(rows, &stored); err != nil {
		return Notification{}, false, err
	}
	return stored, true, rows.Err()
}

const updateNotificationStatement = `
UPDATE notifications
SET read_at = $2, active = $3, version = $4
WHERE id = $1
RETURNING` + notificationFields

func (s *Store) update(ctx context.Context, changed Notification) (Notification, error) {
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, updateNotificationStatement,
		changed.ID, changed.ReadAt, changed.Active, changed.Version)
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
	var stored Notification
	if err = scanNotification(rows, &stored); err != nil {
		return Notification{}, err
	}
	return stored, rows.Err()
}

// change writes a representation that replaced a stored one and reports it as changed.
func (s *Store) change(ctx context.Context, next Notification) (Notification, bool, error) {
	stored, err := s.update(ctx, next)
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
	)
}
