package notifications

import (
	"time"

	"github.com/google/uuid"
)

// Kind is what a notification tells its owner about. The spelling is the one the contract declares,
// so the word a record carries is the word a client reads.
type Kind string

const (
	// ReservationExpiring warns that a reservation has entered its last minute.
	ReservationExpiring Kind = "reservation_expiring"

	// RentalCompleted reports a finished ride together with the bill it produced.
	RentalCompleted Kind = "rental_completed"
)

// initialVersion is the version a notification is created at. The record itself is the first
// representation of the change it announces, and every stored change of that representation — the
// reservation ending, the first read — reaches the next one.
const initialVersion int64 = 1

// Notification is one stored notification of one account: what it tells about, when it was written,
// whether it is still current and how many times its published representation has changed.
type Notification struct {
	ID        string
	UserID    uuid.UUID
	RentalID  string
	Kind      Kind
	CreatedAt time.Time
	ReadAt    *time.Time

	// ExpiresAt is the deadline of the rental the notification is about. It is read from the rental
	// rather than stored here, because a warning publishes the deadline it warns about and a second
	// copy of it could disagree with the rental.
	ExpiresAt time.Time

	// Active is whether what the notification tells about is still in force. Marking a notification
	// read does not change it: having acknowledged a warning is a different fact from the warning
	// having stopped being true.
	Active bool

	// Version counts the stored changes of the published representation, which is what a client
	// compares to tell a newer answer from an older one.
	Version int64
}

// About is a notification about to be created: the account it belongs to, the rental it describes
// and what it tells about that rental.
type About struct {
	UserID   uuid.UUID
	RentalID string
	Kind     Kind
}

// Created returns the notification a creation stores at one moment: active, unread and at its first
// version. The identifier is the storage's own, so two creations that race are told apart by the
// unique key rather than by the order they were written in.
func Created(about About, at time.Time) (Notification, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Notification{}, err
	}
	return Notification{
		ID:        id.String(),
		UserID:    about.UserID,
		RentalID:  about.RentalID,
		Kind:      about.Kind,
		CreatedAt: at,
		Active:    true,
		Version:   initialVersion,
	}, nil
}

// Deactivated returns the representation a deactivation stores, and whether it stores one. What the
// notification told about has stopped being in force, so it moves to the next version; one that is
// already inactive is returned as it stands, because a repeated transition changes nothing.
func (n Notification) Deactivated() (Notification, bool) {
	if !n.Active {
		return n, false
	}
	n.Active = false
	n.Version++
	return n, true
}

// Read returns the representation that marking a notification read stores, and whether it stores
// one. Reading is a different fact from being current: the notification keeps its activity and moves
// to the next version once, so a repeated read changes nothing further.
func (n Notification) Read(at time.Time) (Notification, bool) {
	if n.ReadAt != nil {
		return n, false
	}
	readAt := at
	n.ReadAt = &readAt
	n.Version++
	return n, true
}
