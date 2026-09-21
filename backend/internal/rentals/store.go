package rentals

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store owns the rows of the rentals module. Ordinary module operations use the transaction carried
// by their context; operations exposed to another module require that caller's transaction directly.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// InstallationMetadata is the service-wide calendar and display metadata owned beside the daily
// reservation limit that consumes its timezone.
type InstallationMetadata struct {
	City     string
	Currency string
	Timezone string
}

const installationMetadataSelection = `
SELECT city, currency, timezone
FROM bootstrap_metadata
WHERE singleton`

// Metadata reads the one installation declaration used by both the daily limit and readiness.
func (s *Store) Metadata(ctx context.Context) (InstallationMetadata, error) {
	var metadata InstallationMetadata
	err := database.QuerierFrom(ctx, s.pool).QueryRow(ctx, installationMetadataSelection).Scan(
		&metadata.City,
		&metadata.Currency,
		&metadata.Timezone,
	)
	return metadata, err
}

// rentalFields is the one stored shape of a rental. Every read, insert result and transition result
// is projected through this declaration, so adding a stored field is one change.
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
    exhausted_sources,
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

	liveRentalsSelection = rentalColumns + `
WHERE ended_at IS NULL
ORDER BY vehicle_id`
)

func rentalByID(ctx context.Context, pool *pgxpool.Pool, id string) (Rental, error) {
	return readRental(ctx, pool, rentalByIDSelection, id)
}

func rentalByIDFor(
	ctx context.Context,
	pool *pgxpool.Pool,
	owner uuid.UUID,
	id string,
) (Rental, error) {
	return readRental(ctx, pool, rentalByIDSelection+` AND user_id = $2`, id, owner)
}

func liveRentalOf(
	ctx context.Context,
	pool *pgxpool.Pool,
	selection string,
	identifier any,
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

func readRental(
	ctx context.Context,
	pool *pgxpool.Pool,
	selection string,
	arguments ...any,
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
	var exhausted []string
	err := rows.Scan(
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
		&exhausted,
		&found.ModeStartedAt,
		&found.Tariff.ID,
		&found.Tariff.Currency,
		&found.Tariff.BillingPolicy,
		&found.Tariff.DrivingRateTyiynPerStartedMinute,
		&found.Tariff.PausedRateTyiynPerStartedMinute,
		&found.Tariff.Version,
	)
	if err != nil {
		return err
	}
	found.Exhausted = fleet.SourceKinds(exhausted)
	return nil
}

func liveRentals(ctx context.Context, pool *pgxpool.Pool) ([]Rental, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(ctx, liveRentalsSelection)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	held := []Rental{}
	for rows.Next() {
		var found Rental
		if err = scanRental(rows, &found); err != nil {
			return nil, err
		}
		held = append(held, found)
	}
	return held, rows.Err()
}

const insertReservationStatement = `
INSERT INTO rentals (` + rentalFields + `
)
VALUES (
    $1, $2, $3, $6, $4, $14, $7, $8, NULL, NULL, NULL, NULL, NULL, $5,
    $9, $10, $11, $12, $13
)
ON CONFLICT DO NOTHING`

const reservationInitialVersion int64 = 1

func insertReservation(
	ctx context.Context,
	pool *pgxpool.Pool,
	about reservation,
) (Rental, bool, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Rental{}, false, err
	}
	written, err := database.QuerierFrom(ctx, pool).Exec(
		ctx,
		insertReservationStatement,
		id.String(),
		about.userID,
		about.vehicleID,
		stage.Reserved,
		about.price.ID,
		about.zoneID,
		about.moment,
		about.moment.Add(ReservationLifetime),
		about.price.Currency,
		about.price.BillingPolicy,
		about.price.DrivingRateTyiynPerStartedMinute,
		about.price.PausedRateTyiynPerStartedMinute,
		about.price.Version,
		reservationInitialVersion,
	)
	if err != nil {
		return Rental{}, false, err
	}
	if written.RowsAffected() == 0 {
		return Rental{}, false, nil
	}
	rental, err := rentalByID(ctx, pool, id.String())
	return rental, err == nil, err
}

const startRideStatement = `
UPDATE rentals
SET stage = $2, started_at = $3, mode_started_at = $3, version = version + 1
WHERE id = $1 AND stage = $4
RETURNING` + rentalFields

const changeModeStatement = `
UPDATE rentals
SET stage = $2, mode_started_at = $3, version = version + 1
WHERE id = $1 AND stage = $4
RETURNING` + rentalFields

func moveRide(
	ctx context.Context,
	pool *pgxpool.Pool,
	target Rental,
	transition rideTransition,
	moment time.Time,
) (Rental, error) {
	statement := changeModeStatement
	if transition.from == stage.Reserved {
		statement = startRideStatement
	}
	rows, err := database.QuerierFrom(ctx, pool).Query(
		ctx,
		statement,
		target.ID,
		transition.to,
		moment,
		transition.from,
	)
	if err != nil {
		return Rental{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Rental{}, err
		}
		return Rental{}, errRentalMoved
	}
	var moved Rental
	if err = scanRental(rows, &moved); err != nil {
		return Rental{}, err
	}
	return moved, rows.Err()
}

const releaseRentalStatement = `
UPDATE rentals
SET stage = $3, ended_at = $4, mode_started_at = NULL, version = version + 1
WHERE id = $1 AND stage = $2
RETURNING` + rentalFields

func endReservationAs(
	ctx context.Context,
	pool *pgxpool.Pool,
	id string,
	ending stage.Stage,
	at time.Time,
) (Rental, bool, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(
		ctx,
		releaseRentalStatement,
		id,
		stage.Reserved,
		ending,
		at,
	)
	if err != nil {
		return Rental{}, false, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Rental{}, false, err
		}
		return Rental{}, false, nil
	}
	var ended Rental
	if err = scanRental(rows, &ended); err != nil {
		return Rental{}, false, err
	}
	return ended, true, rows.Err()
}

const completeRentalStatement = `
UPDATE rentals
SET stage = $2::text,
    ended_at = $3,
    mode_started_at = NULL,
    completion_reason = $4::text,
    exhausted_sources = $5::text[],
    version = version + 1
WHERE id = $1 AND stage = ANY($6::text[])
RETURNING` + rentalFields

func completeRental(
	ctx context.Context,
	pool *pgxpool.Pool,
	target Rental,
	ending Ending,
) (Rental, error) {
	rows, err := database.QuerierFrom(ctx, pool).Query(
		ctx,
		completeRentalStatement,
		target.ID,
		string(stage.Completed),
		ending.EndedAt,
		string(ending.Reason),
		fleet.SourceNames(ending.Exhausted),
		rideStages,
	)
	if err != nil {
		return Rental{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Rental{}, err
		}
		return Rental{}, errRentalNotEnded
	}
	var ended Rental
	if err = scanRental(rows, &ended); err != nil {
		return Rental{}, err
	}
	return ended, rows.Err()
}

// PreparedRental is one rental the demonstration installs or restores.
type PreparedRental struct {
	ID        string
	UserID    uuid.UUID
	VehicleID string
	Stage     stage.Stage
	TariffID  string
	ZoneID    string
}

const insertPreparedRentalStatement = `
INSERT INTO rentals (` + rentalFields + `
)
SELECT $1, $2, $3, $6, $4, $9, now(), now() + make_interval(secs => $7),
       CASE WHEN $8 THEN now() END,
       NULL,
       NULL,
       NULL,
       CASE WHEN $8 THEN now() END,
       $5,
       price.currency, price.billing_policy, price.driving_rate_tyiyn_per_started_minute,
       price.paused_rate_tyiyn_per_started_minute, price.version
FROM tariffs price
WHERE price.id = $5
ON CONFLICT DO NOTHING`

// InstallPrepared creates one prepared rental if no uniqueness constraint already claims it.
func (s *Store) InstallPrepared(ctx context.Context, tx pgx.Tx, prepared PreparedRental) error {
	_, err := tx.Exec(
		ctx,
		insertPreparedRentalStatement,
		prepared.ID,
		prepared.UserID,
		prepared.VehicleID,
		prepared.Stage,
		prepared.TariffID,
		prepared.ZoneID,
		ReservationLifetime.Seconds(),
		prepared.Stage != stage.Reserved,
		reservationInitialVersion,
	)
	return err
}

const restorePreparedRentalStatement = `
INSERT INTO rentals (` + rentalFields + `
)
SELECT $1, $2, $3, $6, $4, $9, now(), now() + make_interval(secs => $7),
       CASE WHEN $8 THEN now() END,
       NULL,
       NULL,
       NULL,
       CASE WHEN $8 THEN now() END,
       $5,
       price.currency, price.billing_policy, price.driving_rate_tyiyn_per_started_minute,
       price.paused_rate_tyiyn_per_started_minute, price.version
FROM tariffs price
WHERE price.id = $5
ON CONFLICT (id) DO UPDATE SET
    stage = EXCLUDED.stage,
    tariff_id = EXCLUDED.tariff_id,
    zone_id = EXCLUDED.zone_id,
    reserved_at = EXCLUDED.reserved_at,
    expires_at = EXCLUDED.expires_at,
    started_at = EXCLUDED.started_at,
    mode_started_at = EXCLUDED.mode_started_at,
    ended_at = CASE WHEN EXCLUDED.stage = 'completed' THEN now() END,
    completion_reason = CASE WHEN EXCLUDED.stage = 'completed' THEN rentals.completion_reason END,
    exhausted_sources = CASE WHEN EXCLUDED.stage = 'completed' THEN rentals.exhausted_sources END,
    tariff_currency = EXCLUDED.tariff_currency,
    tariff_billing_policy = EXCLUDED.tariff_billing_policy,
    tariff_driving_rate_tyiyn_per_started_minute = EXCLUDED.tariff_driving_rate_tyiyn_per_started_minute,
    tariff_paused_rate_tyiyn_per_started_minute = EXCLUDED.tariff_paused_rate_tyiyn_per_started_minute,
    tariff_version = EXCLUDED.tariff_version,
    version = rentals.version + 1
RETURNING version`

// RestorePrepared returns one prepared rental to its declared live state and reports the version it
// reached. Existing history on that row is cleared exactly when the declared state is live.
func (s *Store) RestorePrepared(
	ctx context.Context,
	tx pgx.Tx,
	prepared PreparedRental,
) (int64, error) {
	var version int64
	err := tx.QueryRow(
		ctx,
		restorePreparedRentalStatement,
		prepared.ID,
		prepared.UserID,
		prepared.VehicleID,
		prepared.Stage,
		prepared.TariffID,
		prepared.ZoneID,
		ReservationLifetime.Seconds(),
		prepared.Stage != stage.Reserved,
		reservationInitialVersion,
	).Scan(&version)
	return version, err
}

const scenarioRentalsSelection = `
SELECT id, user_id
FROM rentals
WHERE vehicle_id = ANY($1)
ORDER BY id`

type scenarioParticipantSource struct {
	pool       *pgxpool.Pool
	vehicleIDs []string
	userIDs    []uuid.UUID
}

func (s scenarioParticipantSource) discover(ctx context.Context) (participants, error) {
	planned := participants{
		users:    append([]uuid.UUID(nil), s.userIDs...),
		vehicles: sortedIdentifiers(s.vehicleIDs...),
	}
	rows, err := database.QuerierFrom(ctx, s.pool).Query(ctx, scenarioRentalsSelection, s.vehicleIDs)
	if err != nil {
		return participants{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var rentalID string
		var userID uuid.UUID
		if err = rows.Scan(&rentalID, &userID); err != nil {
			return participants{}, err
		}
		planned.rentals = append(planned.rentals, rentalID)
		planned.users = append(planned.users, userID)
	}
	planned.users = distinctUsers(planned.users)
	planned.rentals = sortedIdentifiers(planned.rentals...)
	return planned, rows.Err()
}

// WithScenarioRowsLocked runs a demonstration restoration under the shared rental lock plan.
// Relationships are checked again after locking, so a new rental that raced the restoration makes
// the attempt restart instead of adding a late lock out of order.
func WithScenarioRowsLocked(
	ctx context.Context,
	pool *pgxpool.Pool,
	vehicleIDs []string,
	scenarioUserIDs []uuid.UUID,
	work func(context.Context, pgx.Tx) error,
) error {
	participants := scenarioParticipantSource{
		pool:       pool,
		vehicleIDs: vehicleIDs,
		userIDs:    scenarioUserIDs,
	}
	return transact(
		ctx,
		pool,
		participants.discover,
		func(txCtx context.Context, tx pgx.Tx, _ time.Time) error {
			return work(txCtx, tx)
		},
	)
}

const vehiclesRentedByPeopleSelection = `
SELECT DISTINCT vehicle_id
FROM rentals
WHERE vehicle_id = ANY($1) AND NOT (user_id = ANY($2))
ORDER BY vehicle_id`

// VehiclesRentedByPeople reports the scenario vehicles touched by an account outside the scenario.
func (s *Store) VehiclesRentedByPeople(
	ctx context.Context,
	tx pgx.Tx,
	vehicleIDs []string,
	scenarioUserIDs []uuid.UUID,
) ([]string, error) {
	rows, err := tx.Query(ctx, vehiclesRentedByPeopleSelection, vehicleIDs, scenarioUserIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicting []string
	for rows.Next() {
		var vehicleID string
		if err = rows.Scan(&vehicleID); err != nil {
			return nil, err
		}
		conflicting = append(conflicting, vehicleID)
	}
	return conflicting, rows.Err()
}

const deleteScenarioRentalsStatement = `
DELETE FROM rentals WHERE vehicle_id = ANY($1) AND NOT (id = ANY($2))`

// DeleteScenarioRentals removes rows on scenario vehicles except those the declaration restores.
func (s *Store) DeleteScenarioRentals(
	ctx context.Context,
	tx pgx.Tx,
	vehicleIDs []string,
	preparedIDs []string,
) error {
	_, err := tx.Exec(ctx, deleteScenarioRentalsStatement, vehicleIDs, preparedIDs)
	return err
}

// Each participant statement orders the rows inside its own group.
const (
	lockUsersStatement    = `SELECT id FROM users WHERE id = ANY($1) ORDER BY id FOR UPDATE`
	lockVehiclesStatement = `SELECT id FROM vehicles WHERE id = ANY($1) ORDER BY id FOR UPDATE`
	lockRentalsStatement  = `SELECT id FROM rentals WHERE id = ANY($1) ORDER BY id FOR UPDATE`
)

const momentStatement = `SELECT clock_timestamp()`

const dueReservationsStatement = `
SELECT id
FROM rentals
WHERE stage = $1 AND expires_at <= clock_timestamp()
ORDER BY expires_at, id`

const dueWarningsStatement = `
SELECT id
FROM rentals
WHERE stage = $1
  AND expires_at > clock_timestamp()
  AND expires_at - make_interval(secs => $2) <= clock_timestamp()
ORDER BY expires_at, id`

const dailyLimitStatement = `
SELECT
    NOT EXISTS (
        SELECT 1
        FROM rentals rental
        WHERE rental.user_id = $1
          AND (rental.reserved_at AT TIME ZONE $3::text)::date
              = ($2::timestamptz AT TIME ZONE $3::text)::date
    ),
    (date_trunc('day', $2::timestamptz AT TIME ZONE $3::text) + interval '1 day')
        AT TIME ZONE $3::text`

const modeDurationsStatement = `
SELECT mode,
       COALESCE(sum(COALESCE(ended_at, $2::timestamptz) - started_at), '0'::interval)
FROM ride_segments
WHERE rental_id = $1 AND started_at <= $2
GROUP BY mode`

const closeOpenSegmentStatement = `
UPDATE ride_segments
SET ended_at = $2
WHERE rental_id = $1 AND ended_at IS NULL`

const openSegmentStatement = `
INSERT INTO ride_segments (rental_id, mode, started_at) VALUES ($1, $2, $3)`

const recordPaymentDemandStatement = `
INSERT INTO demo_payment_outcomes (rental_id, outcome, set_at)
VALUES ($1, $2, $3)
ON CONFLICT (rental_id) DO UPDATE SET outcome = EXCLUDED.outcome, set_at = EXCLUDED.set_at`

const ridePageSelection = `
SELECT rental.id,
       rental.started_at,
       rental.ended_at,
       rental.completion_reason,
       rental.exhausted_sources,
       vehicle.id,
       vehicle.model,
       vehicle.powertrain_type,
       invoice.id
FROM rentals rental
JOIN vehicles vehicle ON vehicle.id = rental.vehicle_id
LEFT JOIN invoices invoice ON invoice.rental_id = rental.id
WHERE rental.user_id = $1
  AND rental.stage = 'completed'
  AND rental.started_at IS NOT NULL
  AND (rental.ended_at, rental.id) <
      (COALESCE($2::timestamptz, 'infinity'::timestamptz),
       COALESCE($3::uuid, '00000000-0000-0000-0000-000000000000'::uuid))
ORDER BY rental.ended_at DESC, rental.id DESC
LIMIT $4`
