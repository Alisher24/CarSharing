package notifications

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// CreateReservationWarning records the warning caused by a reservation entering its last minute.
func (s *Store) CreateReservationWarning(
	ctx context.Context,
	owner uuid.UUID,
	rentalID string,
	at time.Time,
) (bool, error) {
	_, created, err := s.Create(ctx, About{
		UserID:   owner,
		RentalID: rentalID,
		Kind:     ReservationExpiring,
	}, at)
	return created, err
}

// EndReservationWarning makes a reservation warning no longer current.
func (s *Store) EndReservationWarning(ctx context.Context, rentalID string) error {
	_, _, err := s.Deactivate(ctx, rentalID, ReservationExpiring)
	return err
}
