// What the reservation suites need beyond one HTTP call: the two private paths, the command keys a
// mutation requires, the queries that read what the database actually holds, and the barrier that
// starts several requests at the same moment.
import { randomUUID } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import { call, compose, registerAccount, resetRateLimits, serviceOrigin, sql } from './client.mjs';

export const RESERVATIONS_PATH = '/api/v1/reservations';
export const CURRENT_PATH = '/api/v1/me/current';
export const VEHICLES_PATH = '/api/v1/vehicles';
export const IDEMPOTENCY_HEADER = 'Idempotency-Key';

/** How long a check waits for a condition the service reaches on its own. */
export const PATIENCE_MS = 20_000;

/** How often a check asks again while it waits. */
const POLL_MS = 100;

/** A fresh command key. The contract fixes its shape: a canonical unquoted UUID v4. */
export function newCommandKey() {
  return randomUUID();
}

/**
 * race runs one action per participant at the same moment. Every participant waits on the same
 * promise, so the requests arrive together rather than one after the previous answer, which is what
 * makes a race a race.
 */
export async function race(participants) {
  let start;
  const gate = new Promise((resolve) => {
    start = resolve;
  });
  const running = participants.map(async (participant) => {
    await gate;
    return participant();
  });
  start();
  return Promise.all(running);
}

/**
 * The accounts and reservations this suite made, so that it can end them before it finishes. The
 * demonstration the suites after it read is the prepared one, and a rental a check left behind would
 * stop the scenario from being put back at all.
 */
const madeAccounts = [];
const madeRentals = [];

/**
 * How many registrations one address may make before the service refuses the next one. The suites
 * share one address, and clearing the counters is the harness standing in for the passage of time,
 * which is also how access returns in production.
 */
const REGISTRATIONS_BEFORE_RESET = 8;

/** Registers a fresh account, which is the only way these suites obtain a signed-in person. */
export async function newAccount(prefix = 'reservations') {
  if (madeAccounts.length % REGISTRATIONS_BEFORE_RESET === 0) resetRateLimits();

  const account = await registerAccount(prefix);
  madeAccounts.push(account);
  return account;
}

/** Places one reservation and reports the answer, whatever it is. */
export async function reserve(vehicleId, key, account) {
  const answer = await call(RESERVATIONS_PATH, {
    method: 'POST',
    body: { vehicle_id: vehicleId },
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: key },
  });
  if (answer.status === 201) madeRentals.push({ id: answer.json.rental.id, account });
  return answer;
}

/** Gives one reservation back and reports the answer, whatever it is. */
export async function cancel(rentalId, key, account) {
  return call(`${RESERVATIONS_PATH}/${rentalId}/cancel`, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: key },
  });
}

/** Reads the current rental and the day's allowance of one account. */
export function currentOf(account) {
  return call(CURRENT_PATH, { cookie: account.cookie });
}

/**
 * endSuiteReservations ends everything this suite still holds. Each reservation is given back
 * through the service first, so the release is the one the API performs; the rows the suite made are
 * then removed, because the demonstration refuses to be put back while a rental of a person's — ended
 * or not — stands on one of its vehicles, and the suites that follow put it back.
 */
export async function endSuiteReservations() {
  for (const made of madeRentals) {
    await cancel(made.id, newCommandKey(), made.account).catch(() => undefined);
  }
  madeRentals.length = 0;
  if (madeAccounts.length === 0) return;

  const mine = `(SELECT id FROM users WHERE email IN (${madeAccounts.map((one) => `'${one.email}'`).join(', ')}))`;
  sql(`DELETE FROM outbox WHERE resource_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  // A ride a suite finished left an invoice and the report of it behind. Both belong to the rental they
  // describe, and the notification also names the invoice, so the invoice goes before the rental it
  // belongs to.
  sql(`DELETE FROM invoices WHERE rental_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
}

/** Puts the prepared demonstration back, which a reservation of a person's would otherwise refuse. */
export function restoreScenario() {
  return compose('--profile', 'demo', 'run', '--rm', 'demo-scenario');
}

/** One vehicle the public catalog publishes as free to take, or several of them. */
export async function availableVehicles(count = 1) {
  const answer = await call(VEHICLES_PATH);
  const free = answer.json.items.filter((vehicle) => vehicle.status === 'available');
  if (free.length < count) throw new Error(`the fleet published fewer than ${count} available vehicles`);
  return free.slice(0, count).map((vehicle) => vehicle.id);
}

export async function availableVehicle() {
  const [first] = await availableVehicles(1);
  return first;
}

/** One vehicle the catalog publishes in the given state. */
export async function vehicleWithStatus(status) {
  const answer = await call(VEHICLES_PATH);
  const found = answer.json.items.find((vehicle) => vehicle.status === status);
  if (found === undefined) throw new Error(`the fleet published no ${status} vehicle`);
  return found;
}

/** One vehicle as the catalog publishes it now. */
export async function publishedVehicle(vehicleId) {
  const answer = await call(`${VEHICLES_PATH}/${vehicleId}`);
  if (answer.status !== 200) throw new Error(answer.text);
  return answer.json;
}

/**
 * What the database holds for one rental: its stage, version, owner, vehicle, the three moments,
 * the frozen rates and the interval between the reserved moment and the deadline.
 */
export function storedRental(rentalId) {
  const answer = sql(
    `SELECT stage, version, user_id, vehicle_id, ` +
      `to_char(reserved_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), ` +
      `to_char(expires_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), ` +
      `coalesce(to_char(ended_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), ''), ` +
      `tariff_driving_rate_tyiyn_per_started_minute, tariff_paused_rate_tyiyn_per_started_minute, ` +
      `(expires_at - reserved_at) ` +
      `FROM rentals WHERE id = '${rentalId}'`,
  );
  return answer.split('|').map((value) => value.trim());
}

/** How many live rentals one vehicle or one account holds, which a race must not double. */
export function liveRentals(column, value) {
  return Number(sql(`SELECT count(*) FROM rentals WHERE ${column} = '${value}' AND ended_at IS NULL`));
}

/** How many reservations one account has made on the service-timezone day of a moment. */
export function reservationsOnDay(userId, at = 'now()') {
  return Number(
    sql(
      `SELECT count(*) FROM rentals rental, bootstrap_metadata meta ` +
        `WHERE meta.singleton AND rental.user_id = '${userId}' ` +
        `AND (rental.reserved_at AT TIME ZONE meta.timezone)::date ` +
        `= ((${at})::timestamptz AT TIME ZONE meta.timezone)::date`,
    ),
  );
}

/** Every task the outbox holds for one resource, so a check can prove a change was announced. */
export function outboxTasksFor(resourceId) {
  return Number(sql(`SELECT count(*) FROM outbox WHERE resource_id = '${resourceId}'`));
}

/** How many tasks of one kind the outbox addresses to one account. */
export function outboxTasksAddressedTo(userId, kind) {
  return Number(sql(`SELECT count(*) FROM outbox WHERE recipient_id = '${userId}' AND kind = '${kind}'`));
}

/** How many command results one account holds, which a rollback must leave at none. */
export function storedResults(userId) {
  return Number(sql(`SELECT count(*) FROM idempotency_requests WHERE user_id = '${userId}'`));
}

/** The identifier of the account behind one address, which no private answer publishes. */
export function accountId(email) {
  return sql(`SELECT id FROM users WHERE email = '${email}'`);
}

/** The version one vehicle is published at, which a refusal must leave alone. */
export function vehicleVersion(vehicleId) {
  return Number(sql(`SELECT version FROM vehicles WHERE id = '${vehicleId}'`));
}

/** Moves one reservation's deadline, which is how a check reaches a boundary without waiting. */
export function moveDeadline(rentalId, seconds) {
  sql(
    `UPDATE rentals SET expires_at = clock_timestamp() + make_interval(secs => ${seconds}) ` +
      `WHERE id = '${rentalId}'`,
  );
}

/** Dates one reservation to the given moment, which is how a check reaches another service day. */
export function dateReservation(rentalId, moment) {
  sql(`UPDATE rentals SET reserved_at = ${moment} WHERE id = '${rentalId}'`);
}

/** Waits until a condition holds, and fails with what it saw when it never does. */
export async function until(reached, complaint, patienceMs = PATIENCE_MS) {
  const deadline = Date.now() + patienceMs;
  for (;;) {
    const value = await reached();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`${complaint} within ${patienceMs} ms`);
    await delay(POLL_MS);
  }
}

export { call, compose, serviceOrigin, sql };
