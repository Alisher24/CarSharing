// What the payment suites need beyond one HTTP call: the path the payment is sent to, the queries that
// read what the database holds about a payment, and the one way a demonstration asks for an outcome.
//
// A demand for the outcome of the next attempt is written as the migrator role, which is the role that
// owns the schema: the application reads that table and spends a demand, and is granted no right to
// create one. That is the whole of the protected demonstration scenario in this build — the HTTP
// surface of the demonstration control is not served — so a check that needs a decline writes the
// demand the way the demonstration control would.
import { call, sql } from './client.mjs';
import { endSuiteReservations } from './reservations.mjs';

/** Where a payment is sent, which is the path its idempotency fingerprint covers. */
export const payPath = (invoiceId) => `/api/v1/me/invoices/${invoiceId}/pay`;

/** The kind of task the ending of a ride owes the first attempt at its invoice. */
export const PAYMENT_ATTEMPT = 'payment.attempt';

/** The signal every payment transition announces, which no other transition of this build writes. */
export const INVOICE_CHANGED = 'invoice.changed';

/** The outcomes a demand can ask for, which is also the vocabulary the table admits. */
export const DEMO_PAID = 'paid';
export const DEMO_FAILED = 'failed';

/**
 * One payment of the caller's own invoice. The contract forbids a body on this operation, so none is
 * sent; the key is what makes a repeat the same command rather than a second payment.
 */
export function pay(invoiceId, key, account, options = {}) {
  return call(payPath(invoiceId), {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { 'Idempotency-Key': key },
    ...options,
  });
}

/**
 * Asks the next attempt at one ride to end in the given outcome. A demand is one-shot: the attempt it
 * decides spends it, so a check that wants a second decline records a second demand.
 */
export function demandOutcome(rentalId, outcome) {
  sql(
    `INSERT INTO demo_payment_outcomes (rental_id, outcome, set_at)
     VALUES ('${rentalId}', '${outcome}', clock_timestamp())
     ON CONFLICT (rental_id) DO UPDATE
     SET outcome = excluded.outcome, set_at = excluded.set_at`,
  );
}

/** Whether a demand is still waiting to decide an attempt. */
export function demandOf(rentalId) {
  return sql(`SELECT coalesce((SELECT outcome FROM demo_payment_outcomes WHERE rental_id = '${rentalId}'), '')`);
}

/**
 * What the database holds for one payment: its state, the version of the view it is published through,
 * the moment of that state and the reason a refusal carries, each as the text of the row. A moment that
 * is not there is written as nothing rather than as a moment of the year one.
 */
export function storedPayment(invoiceId) {
  const row = sql(
    `SELECT status || '|' || version || '|' ||
            coalesce(to_char(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '') || '|' ||
            coalesce(to_char(paid_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '') || '|' ||
            coalesce(to_char(failed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '') || '|' ||
            coalesce(failure_code, '')
     FROM invoice_payments WHERE invoice_id = '${invoiceId}'`,
  );
  const [status, version, updatedAt, paidAt, failedAt, failureCode] = row.split('|');
  return { status, version: Number(version), updatedAt, paidAt, failedAt, failureCode };
}

/** How many payments one invoice holds, which a repeated attempt must not add to. */
export function paymentCount(invoiceId) {
  return Number(sql(`SELECT count(*) FROM invoice_payments WHERE invoice_id = '${invoiceId}'`));
}

/**
 * How many tasks of one kind the queue holds for one resource. A signal that was delivered is deleted
 * by the retention sweep once its day has passed, so a check that asks what a transition announced
 * reads this immediately afterwards; a task that was never delivered stays.
 */
export function tasksFor(kind, resourceId) {
  return Number(sql(`SELECT count(*) FROM outbox WHERE kind = '${kind}' AND resource_id = '${resourceId}'`));
}

/** How many attempts one invoice is still owed: its tasks of that kind that no worker has delivered. */
export function awaitingAttempt(invoiceId) {
  return Number(
    sql(
      `SELECT count(*) FROM outbox
       WHERE kind = '${PAYMENT_ATTEMPT}' AND resource_id = '${invoiceId}' AND completed_at IS NULL`,
    ),
  );
}

/** The amount one invoice states, which no payment transition moves. */
export function storedTotal(invoiceId) {
  return sql(`SELECT total_amount_tyiyn FROM invoices WHERE id = '${invoiceId}'`);
}

/** The moment one invoice was issued, which a zero invoice is settled by. */
export function issuedAt(invoiceId) {
  return sql(
    `SELECT to_char(issued_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
     FROM invoices WHERE id = '${invoiceId}'`,
  );
}

/** Every line of one invoice as it is stored, so a check can prove a refusal left them alone. */
export function storedLines(invoiceId) {
  return sql(
    `SELECT driving_duration_microseconds || '|' || driving_billed_started_minutes || '|' ||
            driving_rate_tyiyn_per_started_minute || '|' ||
            paused_duration_microseconds || '|' || paused_billed_started_minutes || '|' ||
            paused_rate_tyiyn_per_started_minute
     FROM invoices WHERE id = '${invoiceId}'`,
  );
}

/**
 * What a ride became when it ended: its stage, the moment it ended, why it ended and its version. A
 * payment must leave every one of these as the ending wrote them — refusing a payment does not
 * un-finish a ride — so a check compares the whole answer rather than one field of it.
 */
export function storedRide(rentalId) {
  return sql(
    `SELECT stage || '|' ||
            coalesce(to_char(ended_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '') || '|' ||
            coalesce(completion_reason, '') || '|' || version
     FROM rentals WHERE id = '${rentalId}'`,
  );
}

/** How many free reservations one account spent on the service-timezone day of a moment. */
export function dayLimitOf(account) {
  return Number(
    sql(
      `SELECT count(*) FROM rentals rental, bootstrap_metadata meta
       WHERE meta.singleton AND rental.user_id = (SELECT id FROM users WHERE email = '${account.email}')
         AND (rental.reserved_at AT TIME ZONE meta.timezone)::date
           = (now() AT TIME ZONE meta.timezone)::date`,
    ),
  );
}

/**
 * Gives each interval of one paused ride the duration a check states, keeping the chain continuous and
 * the moment the rental publishes as its current mode naming the open interval. The moments are written
 * rather than waited for, and the ride is finished immediately afterwards, so the invoice prices the
 * durations the check stated rather than the wall-clock time the check took.
 */
export function prepareIntervals(rentalId, drivingSeconds, pausedSeconds) {
  const seconds = `CASE segment.mode
                WHEN 'driving' THEN ${drivingSeconds}::double precision
                ELSE ${pausedSeconds}::double precision
              END`;
  sql(
    `WITH ride AS (
       SELECT segment.id,
              segment.ended_at IS NULL AS open,
              row_number() OVER (ORDER BY segment.started_at, segment.id) AS position,
              ${seconds} AS seconds
       FROM ride_segments segment
       WHERE segment.rental_id = '${rentalId}'
     ),
     chained AS (
       SELECT ride.id, ride.open, ride.seconds,
              sum(ride.seconds) OVER (ORDER BY ride.position ROWS UNBOUNDED PRECEDING) AS finish
       FROM ride
     ),
     anchored AS (
       SELECT chained.id, chained.open,
              now() - make_interval(secs => (SELECT max(finish) FROM chained))
                    + make_interval(secs => chained.finish - chained.seconds) AS started_at,
              now() - make_interval(secs => (SELECT max(finish) FROM chained))
                    + make_interval(secs => chained.finish) AS ended_at
       FROM chained
     )
     UPDATE ride_segments AS segment
     SET started_at = anchored.started_at,
         ended_at = CASE WHEN anchored.open THEN NULL ELSE anchored.ended_at END
     FROM anchored
     WHERE segment.id = anchored.id;

     UPDATE rentals
     SET mode_started_at = (
       SELECT started_at FROM ride_segments WHERE rental_id = '${rentalId}' AND ended_at IS NULL
     )
     WHERE id = '${rentalId}'`,
  );
}

/**
 * Moves the rates one rental was reserved under, which is how a check prices a ride past the exact
 * range of a double without waiting for one, and answers what the rental stored before.
 */
export function moveRentalRates(rentalId, drivingRateTyiyn, pausedRateTyiyn) {
  const replaced = storedRates(rentalId);
  sql(
    `UPDATE rentals SET
       tariff_driving_rate_tyiyn_per_started_minute = ${drivingRateTyiyn},
       tariff_paused_rate_tyiyn_per_started_minute = ${pausedRateTyiyn}
     WHERE id = '${rentalId}'`,
  );
  return replaced;
}

/** Puts back the rates moveRentalRates replaced. */
export function restoreRentalRates(rentalId, replaced) {
  sql(
    `UPDATE rentals SET
       tariff_driving_rate_tyiyn_per_started_minute = ${replaced.driving},
       tariff_paused_rate_tyiyn_per_started_minute = ${replaced.paused}
     WHERE id = '${rentalId}'`,
  );
}

/** The rates one rental was reserved under, which is what the invoice of its ride states. */
export function storedRates(rentalId) {
  const rates = sql(
    `SELECT tariff_driving_rate_tyiyn_per_started_minute || '|' ||
            tariff_paused_rate_tyiyn_per_started_minute
     FROM rentals WHERE id = '${rentalId}'`,
  );
  const [driving, paused] = rates.split('|');
  return { driving: Number(driving), paused: Number(paused) };
}

/**
 * Removes every record one check made about a payment: the tasks the queue owes about its invoices and
 * the demands it recorded. The rides themselves are handed back by the reservation suite's cleanup,
 * which is also what frees the vehicles the checks of the next suite need.
 */
export async function endSuitePayments() {
  forgetPayments();
  await endSuiteReservations();
}

/** Removes what one check left behind, so the next check starts from an invoice nobody has paid. */
export function clearPayments() {
  forgetPayments();
}

/** Forgets the tasks, the demands and the invoices of every account this suite registered. */
function forgetPayments() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(
    `DELETE FROM outbox
     WHERE recipient_id IN ${mine}
        OR resource_id IN (SELECT id FROM invoices WHERE user_id IN ${mine})`,
  );
  sql(`DELETE FROM demo_payment_outcomes`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
}

/** The prefix every account this suite registers carries, which is also how its rows are found. */
export const ACCOUNT_PREFIX = 'payments';

/** The role the application connects as, which is the one whose privileges a check observes. */
const APPLICATION_ROLE = 'carsharing_app';

/**
 * Runs one statement as the role the application connects as and reports whether it was accepted.
 *
 * The privilege under check is the application's rather than the schema owner's, so a statement the
 * migrator runs proves nothing about it: this sets the role inside one transaction, which is rolled
 * back, and answers whether the database refused what was written.
 *
 * The statement never names a row and never commits, so a template that changes nothing is safe to
 * present: what is observed is the privilege, not the effect.
 */
export function asApplication(statement) {
  try {
    sql(`BEGIN; SET LOCAL ROLE ${APPLICATION_ROLE}; ${statement}; ROLLBACK;`);
    return { accepted: true, refusal: '' };
  } catch (failure) {
    return { accepted: false, refusal: String(failure.stderr ?? failure.message) };
  }
}
