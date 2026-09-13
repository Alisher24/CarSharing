// Rental rows written directly by the checks that must reach a state the commands of this build do
// not create yet: a rental on a scenario vehicle, a rental that has already ended, a reservation
// whose deadline has passed. The statement lives here rather than in each suite because a rental
// stores the conditions it was made under, and a suite that left them out would be writing a row the
// module itself could never write.

/** The conditions a rental takes from the price list it names, as the insert reads them. */
const FROZEN_CONDITIONS = `
        price.currency,
        price.billing_policy,
        price.driving_rate_tyiyn_per_started_minute,
        price.paused_rate_tyiyn_per_started_minute,
        price.version`;

/** Those same conditions as the columns they are stored in. */
const FROZEN_COLUMNS = `
        tariff_currency,
        tariff_billing_policy,
        tariff_driving_rate_tyiyn_per_started_minute,
        tariff_paused_rate_tyiyn_per_started_minute,
        tariff_version`;

/** One rental to write, with the moments stated as SQL expressions. */
/**
 * insertRental writes one rental: its identifier, the address of the account it belongs to, the
 * vehicle, the stage, and the moments stated as SQL expressions. The conditions come from the price
 * list in force, so the row says what the operator charged at that moment rather than what a later
 * reader would charge.
 */
export function insertRental(row) {
  const version = row.version ?? 1;
  return `
    INSERT INTO rentals (
        id, user_id, vehicle_id, stage, tariff_id, zone_id,
        reserved_at, expires_at, started_at, ended_at, version,${FROZEN_COLUMNS}
    )
    SELECT
        ${quoted(row.id)},
        (SELECT id FROM users WHERE email = ${quoted(row.email)}),
        ${quoted(row.vehicleId)},
        ${quoted(row.stage)},
        price.id,
        (SELECT id FROM service_zones ORDER BY id LIMIT 1),
        ${row.reservedAt},
        ${row.expiresAt},
        ${row.startedAt ?? 'null'},
        ${row.endedAt ?? 'null'},
        ${version},${FROZEN_CONDITIONS}
    FROM tariffs price
    ORDER BY price.id
    LIMIT 1;`;
}

/**
 * The columns the insert above reads from the price list, for a statement that writes a rental inside
 * a larger query of its own. A suite that needs its own shape states these once and keeps the same
 * conditions.
 */
export const rentalConditions = { columns: FROZEN_COLUMNS, values: FROZEN_CONDITIONS };

/** One value as SQL text. Every caller writes a literal of its own, never anything a client sent. */
function quoted(value) {
  return `'${value}'`;
}
