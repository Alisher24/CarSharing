package notifications

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// The version rule of one notification, checked without a database and without a worker: the record
// itself is version one, and every stored change of the published representation reaches the next
// one exactly once. These are the rules the store's statements write, so what a caller reads back is
// decided here rather than by the order two writers happened to run in.
func TestCreatedNotificationIsActiveUnreadAndAtItsFirstVersion(t *testing.T) {
	at := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC)
	owner := uuid.New()
	rental := uuid.New().String()

	created, err := Created(About{UserID: owner, RentalID: rental, Kind: ReservationExpiring}, at)
	if err != nil {
		t.Fatalf("a notification could not be created: %v", err)
	}
	if created.Version != 1 {
		t.Errorf("a created notification is at version %d, want 1", created.Version)
	}
	if !created.Active {
		t.Error("a created notification is not active")
	}
	if created.ReadAt != nil {
		t.Errorf("a created notification is already read at %s", created.ReadAt)
	}
	if !created.CreatedAt.Equal(at) {
		t.Errorf("a created notification carries %s, want the moment %s", created.CreatedAt, at)
	}
	if created.UserID != owner || created.RentalID != rental || created.Kind != ReservationExpiring {
		t.Errorf("a created notification describes %s/%s/%s, want %s/%s/%s",
			created.UserID, created.RentalID, created.Kind, owner, rental, ReservationExpiring)
	}
	if created.ID == "" {
		t.Error("a created notification has no identifier")
	}
}

// Two creations for the same rental and kind are two different records, which is what the unique key
// refuses: the store answers the one that is already stored instead of writing a second.
func TestCreatedNotificationsOfTheSameRentalAreToldApartByTheirKey(t *testing.T) {
	at := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC)
	about := About{UserID: uuid.New(), RentalID: uuid.New().String(), Kind: ReservationExpiring}

	first, err := Created(about, at)
	if err != nil {
		t.Fatalf("a notification could not be created: %v", err)
	}
	second, err := Created(about, at.Add(time.Minute))
	if err != nil {
		t.Fatalf("a notification could not be created: %v", err)
	}
	if first.ID == second.ID {
		t.Error("two creations of one rental and kind carry the same identifier")
	}
}

// A deactivation makes the warning stop being current and moves the version; a second one writes
// nothing at all, so the stored representation keeps the version it reached.
func TestDeactivationMovesTheVersionOnce(t *testing.T) {
	stored := Notification{Active: true, Version: 1}

	deactivated, changed := stored.Deactivated()
	if !changed {
		t.Fatal("the first deactivation of an active notification changed nothing")
	}
	if deactivated.Active {
		t.Error("a deactivated notification is still active")
	}
	if deactivated.Version != 2 {
		t.Errorf("a deactivated notification is at version %d, want 2", deactivated.Version)
	}
	if stored.Active != true || stored.Version != 1 {
		t.Error("a deactivation changed the notification it was derived from")
	}

	again, changed := deactivated.Deactivated()
	if changed {
		t.Error("a repeated deactivation changed the notification")
	}
	if again.Version != deactivated.Version {
		t.Errorf("a repeated deactivation moved the version to %d", again.Version)
	}
}

// Marking read records the moment and moves the version, and leaves the notification active: the
// person acknowledged the warning, the reservation is still running out.
func TestReadingMovesTheVersionOnceAndKeepsTheWarningCurrent(t *testing.T) {
	at := time.Date(2026, time.September, 13, 10, 14, 30, 0, time.UTC)
	stored := Notification{Active: true, Version: 1}

	read, changed := stored.Read(at)
	if !changed {
		t.Fatal("the first read of an unread notification changed nothing")
	}
	if read.ReadAt == nil || !read.ReadAt.Equal(at) {
		t.Errorf("a read notification carries %v, want the moment %s", read.ReadAt, at)
	}
	if !read.Active {
		t.Error("reading a notification deactivated it")
	}
	if read.Version != 2 {
		t.Errorf("a read notification is at version %d, want 2", read.Version)
	}

	again, changed := read.Read(at.Add(time.Minute))
	if changed {
		t.Error("a repeated read changed the notification")
	}
	if !again.ReadAt.Equal(at) {
		t.Errorf("a repeated read moved the moment to %s", again.ReadAt)
	}
	if again.Version != read.Version {
		t.Errorf("a repeated read moved the version to %d", again.Version)
	}
}

// The two changes compose in the order a reservation goes through them: a warning is read and then
// stops being current, and each of the two moves the version once.
func TestReadingAndDeactivatingAreTwoStoredChanges(t *testing.T) {
	at := time.Date(2026, time.September, 13, 10, 14, 30, 0, time.UTC)
	stored := Notification{Active: true, Version: 1}

	read, _ := stored.Read(at)
	deactivated, changed := read.Deactivated()
	if !changed {
		t.Fatal("deactivating a read notification changed nothing")
	}
	if deactivated.Version != 3 {
		t.Errorf("a read and deactivated notification is at version %d, want 3", deactivated.Version)
	}
	if deactivated.ReadAt == nil || !deactivated.ReadAt.Equal(at) {
		t.Error("a deactivation dropped the moment the notification was read")
	}
	if deactivated.Active {
		t.Error("a deactivated notification is still active")
	}
}
