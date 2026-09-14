package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/completion"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrRentalNotFound reports an identifier no rental of this account carries.
var ErrRentalNotFound = errors.New("no such rental")

// Rental is one rental as this module reads and writes it: the reservation it started as, the ride
// it may become, and the moment it released its vehicle.
type Rental struct {
	ID        string
	UserID    uuid.UUID
	VehicleID string
	Stage     stage.Stage
	Version   int64

	ReservedAt time.Time
	ExpiresAt  time.Time
	StartedAt  *time.Time
	EndedAt    *time.Time

	// CompletionReason is why the ride ended, which exactly the rentals that have ended carry.
	CompletionReason *completion.Reason

	// ZoneID is the service area the vehicle stood in when the reservation was made. It is what a
	// finish is judged against: the ride may not be ended where the area does not cover it.
	ZoneID string

	// ModeStartedAt is the moment the ride entered the mode it is in, which is the moment the interval
	// it is in began. It is set exactly while the rental is a ride that has begun.
	ModeStartedAt *time.Time

	// Tariff is the price list this rental was made under, stored with the rental rather than read
	// from the catalog on demand: a later change of the catalog must not reach a reservation that
	// already exists.
	Tariff tariffs.Tariff
}

// Riding reports whether the rental is a ride that has begun, which is what an answer publishes with a
// mode moment and a progress.
func (r Rental) Riding() bool {
	return (r.Stage == stage.Active || r.Stage == stage.Paused) && r.ModeStartedAt != nil
}

// rentalFields is the shape every rental read and every rental transition returns. One declaration
// keeps a rental read by identifier, by owner or by vehicle, and a rental returned by a transition,
// from drifting apart.
const rentalFields = `
    id,
    user_id,
    vehicle_id,
    zone_id,
    stage,
    version,
    reserved_at,
    expires_at,
    started_at,
    ended_at,
    completion_reason,
    mode_started_at,
    tariff_id,
    tariff_currency,
    tariff_billing_policy,
    tariff_driving_rate_tyiyn_per_started_minute,
    tariff_paused_rate_tyiyn_per_started_minute,
    tariff_version`

const rentalColumns = `
SELECT` + rentalFields + `
FROM rentals`

const (
	rentalByIDSelection = rentalColumns + `
WHERE id = $1`

	userLiveRentalSelection = rentalColumns + `
WHERE user_id = $1 AND ended_at IS NULL`

	vehicleLiveRentalSelection = rentalColumns + `
WHERE vehicle_id = $1 AND ended_at IS NULL`
)

// rentalByID reads one rental by its identifier, whatever account it belongs to.
func rentalByID(ctx context.Context, pool *pgxpool.Pool, id string) (Rental, error) {
	return readRental(ctx, pool, rentalByIDSelection, id)
}

// rentalByIDFor reads one rental the account owns. A rental of another account is reported as
// absent, so a caller cannot use the answer to learn that somebody else's rental exists.
func rentalByIDFor(
	ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, id string,
) (Rental, error) {
	found, err := readRental(ctx, pool, rentalByIDSelection, id)
	if err != nil {
		return Rental{}, err
	}
	if found.UserID != userID {
		return Rental{}, ErrRentalNotFound
	}
	return found, nil
}

// liveRentalOf reads the one rental that still holds a user or a vehicle. The partial unique indexes
// on the table make "the one" a guarantee of the database rather than of this query.
func liveRentalOf(
	ctx context.Context, pool *pgxpool.Pool, selection string, identifier any,
) (*Rental, error) {
	found, err := readRental(ctx, pool, selection, identifier)
	if errors.Is(err, ErrRentalNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &found, nil
}

// readRental reads at most one rental, so that a selection matching several rows is reported as a
// failure of the caller's expectation rather than silently answering the first.
func readRental(
	ctx context.Context, pool *pgxpool.Pool, selection string, arguments ...any,
) (Rental, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, selection, arguments...)
	if err != nil {
		return Rental{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Rental{}, err
		}
		return Rental{}, ErrRentalNotFound
	}
	var found Rental
	if err = scanRental(rows, &found); err != nil {
		return Rental{}, err
	}
	if rows.Next() {
		return Rental{}, errors.New("the selection matched more than one rental")
	}
	return found, rows.Err()
}

func scanRental(rows pgx.Rows, found *Rental) error {
	return rows.Scan(
		&found.ID,
		&found.UserID,
		&found.VehicleID,
		&found.ZoneID,
		&found.Stage,
		&found.Version,
		&found.ReservedAt,
		&found.ExpiresAt,
		&found.StartedAt,
		&found.EndedAt,
		&found.CompletionReason,
		&found.ModeStartedAt,
		&found.Tariff.ID,
		&found.Tariff.Currency,
		&found.Tariff.BillingPolicy,
		&found.Tariff.DrivingRateTyiynPerStartedMinute,
		&found.Tariff.PausedRateTyiynPerStartedMinute,
		&found.Tariff.Version,
	)
}
