// What the history suite needs beyond one HTTP call: the two collection paths, and the rows a check
// reads them against. A collection of a known shape is written directly, because a check cannot ride
// twenty rides: the pages, their order and the refusals a cursor meets are what this suite is about,
// and the commands that produce a real ride are proved by the suites that send them.
import assert from 'node:assert/strict';
import { call } from './client.mjs';
import { insertRentalReturning } from './rentalrows.mjs';
import { endSuiteReservations, newAccount, sql } from './reservations.mjs';

export const RIDES_PATH = '/api/v1/me/rides';
export const INVOICES_PATH = '/api/v1/me/invoices';

/** Every account this suite registers carries this prefix, which is also how its rows are found. */
const ACCOUNT_PREFIX = 'history';

/** The moment the contract publishes, which every moment a page states is compared against. */
export const MOMENT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;

/** The rates and the total the written invoices of this suite state, in whole tyiyn. */
const DRIVING_RATE_TYIYN = 1234;
const DRIVING_STARTED_MINUTES = 2;
const PAUSED_RATE_TYIYN = 321;
const PAUSED_STARTED_MINUTES = 1;
export const WRITTEN_TOTAL_TYIYN =
  DRIVING_RATE_TYIYN * DRIVING_STARTED_MINUTES + PAUSED_RATE_TYIYN * PAUSED_STARTED_MINUTES;

/** One page of the caller's own rides, as the request states it. */
export function ridesOf(account, query = '') {
  return call(`${RIDES_PATH}${query}`, { cookie: account.cookie });
}

/** One page of the caller's own invoices, as the request states it. */
export function invoicesOf(account, query = '') {
  return call(`${INVOICES_PATH}${query}`, { cookie: account.cookie });
}

/** One invoice of the caller, which is the read a foreign identifier is refused by. */
export function invoiceOf(account, invoiceId) {
  return call(`/api/v1/me/invoices/${invoiceId}`, { cookie: account.cookie });
}

/** Registers a fresh account, which is the only way this suite obtains a signed-in person. */
export function newSuiteAccount(name) {
  return newAccount(`${ACCOUNT_PREFIX}-${name}`);
}

/**
 * insertRental writes one rental of a stage this suite needs to prove is left out of the history: a
 * reservation given back, one that ran out, or the one that is still live. The moments are stated
 * relative to the moment the suite measures from, so a check knows the order it wrote.
 */
export function insertRental({ account, stage, endedSecondsAgo, at, vehicleId }) {
  const reservedAt = momentExpression(at, endedSecondsAgo);
  const ended = stage === 'reserved' ? null : `(${reservedAt}) + interval '1 second'`;
  return sql(
    insertRentalReturning(
      {
        id: sql('SELECT uuidv7()'),
        email: account.email,
        vehicleId: vehicleId ?? anyVehicle(),
        stage,
        reservedAt,
        expiresAt: `(${reservedAt}) + interval '15 minutes'`,
        startedAt: stage === 'completed' ? reservedAt : null,
        endedAt: ended,
        completionReason: stage === 'completed' ? 'user_finished' : undefined,
      },
      'id',
    ),
  );
}

/**
 * insertCompletedRide writes one finished ride together with the invoice it was charged by and the
 * first state of that invoice's payment. A ride without an invoice is a row the service refuses to
 * publish, so a check that wants one asks for it by name.
 */
export function insertCompletedRide({ account, endedSecondsAgo, at, withInvoice = true, paid = false }) {
  const rentalId = insertRental({ account, stage: 'completed', endedSecondsAgo, at });
  if (!withInvoice) return { rentalId, invoiceId: '' };

  const invoiceId = insertInvoice({ rentalId, account, endedSecondsAgo, at, paid });
  return { rentalId, invoiceId };
}

/**
 * insertInvoice writes one invoice of one ride and the state of its payment. The conditions come from
 * the rental the invoice describes, exactly as the service copies them, so the row states what the
 * operator charged rather than what a later reader would charge.
 */
function insertInvoice({ rentalId, account, endedSecondsAgo, at, paid }) {
  const issuedAt = momentExpression(at, endedSecondsAgo);
  const invoiceId = sql(
    `WITH written AS (
       INSERT INTO invoices (
         id, rental_id, user_id, issued_at, currency, billing_policy, completion_reason,
         driving_duration_microseconds, driving_billed_started_minutes,
         driving_rate_tyiyn_per_started_minute,
         paused_duration_microseconds, paused_billed_started_minutes,
         paused_rate_tyiyn_per_started_minute, total_amount_tyiyn, version
       )
       SELECT uuidv7(), rental.id, rental.user_id, ${issuedAt},
              rental.tariff_currency, rental.tariff_billing_policy, 'user_finished',
              90000000, ${DRIVING_STARTED_MINUTES}, ${DRIVING_RATE_TYIYN},
              45000000, ${PAUSED_STARTED_MINUTES}, ${PAUSED_RATE_TYIYN},
              ${WRITTEN_TOTAL_TYIYN}, 1
       FROM rentals rental WHERE rental.id = '${rentalId}'
       RETURNING id
     )
     SELECT id FROM written`,
  );
  const status = paid ? 'paid' : 'pending';
  const paidAt = paid ? issuedAt : 'null';
  sql(
    `INSERT INTO invoice_payments (invoice_id, status, version, created_at, updated_at, paid_at)
     VALUES ('${invoiceId}', '${status}', 1, ${issuedAt}, ${issuedAt}, ${paidAt})`,
  );
  assert.equal(ownerOf(account), invoiceOwner(invoiceId), 'the written invoice belongs to another account');
  return invoiceId;
}

/** The order the history publishes one account's rides in, read from the database rather than guessed. */
export function publishedRideOrder(account) {
  return identifiers(
    `SELECT string_agg(rental.id::text, ',' ORDER BY rental.ended_at DESC, rental.id DESC)
     FROM rentals rental
     JOIN users account ON account.id = rental.user_id
     WHERE account.email = '${account.email}' AND rental.stage = 'completed'`,
  );
}

/** The order the collection publishes one account's invoices in. */
export function publishedInvoiceOrder(account) {
  return identifiers(
    `SELECT string_agg(invoice.id::text, ',' ORDER BY invoice.issued_at DESC, invoice.id DESC)
     FROM invoices invoice
     JOIN users account ON account.id = invoice.user_id
     WHERE account.email = '${account.email}'`,
  );
}

/** The identifier of the account behind one address, which no private answer publishes. */
export function ownerOf(account) {
  return sql(`SELECT id FROM users WHERE email = '${account.email}'`);
}

/** Removes everything this suite wrote, so the suites after it read the prepared demonstration. */
export async function endSuiteHistory() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(`DELETE FROM outbox WHERE recipient_id IN ${mine}`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
  await endSuiteReservations();
}

/** The moment a written row carries: the suite's own reference moment less the stated age. */
function momentExpression(at, secondsAgo) {
  const base = at ?? 'clock_timestamp()';
  return `(${base}) - make_interval(secs => ${secondsAgo ?? 0})`;
}

function invoiceOwner(invoiceId) {
  return sql(`SELECT user_id FROM invoices WHERE id = '${invoiceId}'`);
}

function identifiers(selection) {
  const rows = sql(selection);
  return rows === '' ? [] : rows.split(',');
}

/** One vehicle of the prepared demonstration, which a written rental only has to name. */
function anyVehicle() {
  return sql('SELECT id FROM vehicles ORDER BY id LIMIT 1');
}

export { sql };
export { CURSOR_ALPHABET, issueCursor, positionOf, tamper } from './cursors.mjs';
