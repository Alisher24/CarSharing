// The failure matrix of the assembled stack: every service restarted on its own, the database taken
// away and given back, a command that fails inside its transaction, and two payments of one invoice
// arriving together. Each check reads the same rows before and after, so what it proves is that a
// failure changes the data in one way rather than in half of one.
//
// The suite stops and starts services, stops the database and withdraws a privilege, so it must not
// run beside another suite: the acceptance suites run one at a time for exactly this reason.
import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';
import { call, compose, sql, waitForReady } from './client.mjs';
import { invoiceOf, invoicesOf, ridesOf } from './history.mjs';
import { PAYMENT_ATTEMPT, awaitingAttempt, pay } from './payments.mjs';
import {
  IDEMPOTENCY_HEADER,
  availableVehicle,
  newAccount,
  newCommandKey,
  reserve,
  restoreScenario,
  until,
} from './reservations.mjs';

const ACCOUNT_PREFIX = 'failures';

/** The services an operator restarts, each on its own. */
const SERVICES = ['postgres', 'api', 'worker', 'mailstub', 'frontend'];

const STATUS_OK = 200;
const STATUS_CONFLICT = 409;
const STATUS_SERVICE_UNAVAILABLE = 503;

before(waitForReady);

// The suite stops services and takes the database away, so it puts all of that back and removes its
// rows before the prepared demonstration is restored: a run that failed part way through would
// otherwise leave a service stopped and the fleet short of the vehicles the next suite takes.
after(async () => {
  for (const service of SERVICES) compose('start', service);
  await waitForReady();
  endSuite();
  restoreScenario();
});

// The fleet the demonstration prepares is what every check of this suite reserves from, so the run
// starts from the prepared scenario rather than from whatever the last one left. A run that failed
// part way through left rentals on those vehicles, and the restoration refuses while a person holds
// one, so this suite releases its own rows first.
before(() => {
  endSuite();
  restoreScenario();
});

describe('a service restarted on its own leaves what the database holds', () => {
  test('each service, restarted alone, keeps the ride, the history, the invoice and the letter', async () => {
    const { rider, holder, rentalId, invoiceId, heldRentalId } = await prepared();

    for (const service of SERVICES) {
      compose('restart', service);
      await waitForReady();

      const current = await call('/api/v1/me/current', { cookie: holder.cookie });
      assert.equal(current.status, STATUS_OK, `after restarting ${service}: ${current.text}`);
      assert.equal(current.json.rental?.id, heldRentalId, `restarting ${service} lost the rental in force`);

      const history = await ridesOf(rider);
      assert.equal(history.status, STATUS_OK, `after restarting ${service}: ${history.text}`);
      assert.deepEqual(
        history.json.items.map((ride) => ride.id),
        [rentalId],
        `restarting ${service} changed the history`,
      );

      const invoices = await invoicesOf(rider);
      assert.equal(invoices.status, STATUS_OK, `after restarting ${service}: ${invoices.text}`);
      assert.deepEqual(
        invoices.json.items.map((view) => view.invoice.id),
        [invoiceId],
        `restarting ${service} changed the invoices`,
      );

      const invoice = await invoiceOf(rider, invoiceId);
      assert.equal(invoice.status, STATUS_OK, `after restarting ${service}: ${invoice.text}`);
      assert.equal(invoice.json.invoice.total_amount_tyiyn, storedTotal(invoiceId));
    }

    process.stdout.write(`failures: ${SERVICES.length} services restarted one by one, nothing changed\n`);
  });
});

describe('the database taken away and given back', () => {
  test('answers a documented 503 while it is gone and serves again without a restart', async () => {
    const { holder } = await prepared();
    compose('stop', 'postgres');
    try {
      const refused = await call('/api/v1/me/current', { cookie: holder.cookie });
      assert.equal(
        refused.status,
        STATUS_SERVICE_UNAVAILABLE,
        `a read with no database answered ${refused.status}: ${refused.text}`,
      );
      assert.equal(refused.json.code, 'SERVICE_UNAVAILABLE', refused.text);
      // The body names the request and nothing else: no driver text and no connection string.
      assert.deepEqual(Object.keys(refused.json).sort(), ['code', 'message', 'request_id']);
      assert.ok(!/postgres|password|dsn|host=/i.test(refused.text), `the refusal leaked a detail: ${refused.text}`);
    } finally {
      compose('start', 'postgres');
    }
    await waitForReady();

    const served = await call('/api/v1/me/current', { cookie: holder.cookie });
    assert.equal(served.status, STATUS_OK, `the API did not serve again: ${served.text}`);
  });
});

describe('a command that fails inside its transaction', () => {
  test('leaves no invoice, no task, no notification and no saved result', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-rollback`);
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);
    const rentalId = created.json.rental.id;
    const userId = accountId(account);

    const started = await rideCommand(`/api/v1/reservations/${rentalId}/start`, account);
    assert.equal(started.status, STATUS_OK, started.text);

    // What the ride already holds before the ending is attempted: the signals of starting it. The
    // ending must add nothing to any of these when it fails in the middle. The starting ride's own
    // deliveries are finished first, because a task still in flight would be completed while the
    // ending is refused and the queue would look shorter for a reason that is not the refusal.
    await until(() => unfinishedTasksOf(rentalId) === 0, 'the starting ride was never delivered');
    const before = stateOfRide(userId, rentalId);

    // The ending writes the ride, the invoice, its lines, the notification, the signals and the saved
    // result in one transaction. Withdrawing one privilege the ending needs fails it in the middle.
    sql('REVOKE INSERT ON invoices FROM carsharing_app');
    let refused;
    try {
      refused = await rideCommand(`/api/v1/rides/${rentalId}/finish`, account);
    } finally {
      sql('GRANT INSERT ON invoices TO carsharing_app');
    }
    assert.equal(
      refused.status,
      STATUS_SERVICE_UNAVAILABLE,
      `the refused ending answered ${refused.status}: ${refused.text}`,
    );

    const after = stateOfRide(userId, rentalId);
    assert.deepEqual(after, before, 'a refused ending left part of itself behind');
    assert.equal(after.stage, 'active', 'the refused ending moved the ride');
    assert.equal(after.invoices, 0, 'a refused ending issued an invoice');
    assert.equal(after.notifications, 0, 'a refused ending wrote a notification');
    assert.equal(after.savedResults, before.savedResults, 'a refused ending saved a result');

    // The command may be made again: nothing of the failed attempt holds the key or the ride.
    const finished = await rideCommand(`/api/v1/rides/${rentalId}/finish`, account);
    assert.equal(finished.status, STATUS_OK, `the command could not be made again: ${finished.text}`);
    const ended = stateOfRide(userId, rentalId);
    assert.equal(ended.stage, 'completed');
    assert.equal(ended.invoices, 1, 'the second ending issued no invoice');
    process.stdout.write(`failures: a refused ending left nothing, and the command could be made again\n`);
  });
});

describe('two payments of one invoice arriving together', () => {
  test('write one payment row and one transition, and neither settles the invoice by itself', async () => {
    const { rider, invoiceId } = await prepared();
    assert.equal(
      count(`SELECT count(*) FROM invoice_payments WHERE invoice_id = '${invoiceId}'`),
      1,
      'the invoice has no single payment row',
    );

    const answers = await Promise.all([pay(invoiceId, newCommandKey(), rider), pay(invoiceId, newCommandKey(), rider)]);
    const statuses = answers.map((answer) => answer.status);

    assert.ok(
      statuses.every((status) => [STATUS_OK, STATUS_CONFLICT].includes(status)),
      `a payment answered something else: ${statuses.join(',')} ${answers.map((one) => one.text).join(' | ')}`,
    );
    // One invoice carries one payment row whatever happens: the table decides it, not a read. The two
    // answers were asked the same question, so both must describe that one row — the same state and the
    // same settled moment — rather than two views of two payments.
    assert.equal(
      count(`SELECT count(*) FROM invoice_payments WHERE invoice_id = '${invoiceId}'`),
      1,
      'two payments were written for one invoice',
    );
    const accepted = answers.filter((answer) => answer.status === STATUS_OK);
    const storedMoment = storedPaidAt(invoiceId);
    const states = accepted.map((answer) => answer.json.invoice.payment.status);
    const moments = accepted.map((answer) => answer.json.invoice.payment.paid_at ?? null);
    for (const moment of moments) {
      assert.equal(moment, storedMoment, 'an answer named a settled moment the row does not hold');
    }
    assert.ok(new Set(states).size <= 1, `two payments of one invoice disagree about its state: ${states.join(',')}`);
    assert.equal(
      count(`SELECT count(*) FROM outbox WHERE kind = '${PAYMENT_ATTEMPT}' AND resource_id = '${invoiceId}'`),
      1,
      'the queue does not hold exactly one attempt for the invoice',
    );
    assert.ok(awaitingAttempt(invoiceId) <= 1, 'the invoice owes more than one attempt');
    process.stdout.write(
      `failures: two payments of one invoice answered ${statuses.join(',')}, ` +
        `one row at ${storedMoment ?? 'no settled moment'}, awaiting=${awaitingAttempt(invoiceId)}\n`,
    );
  });
});

/** One account that rode and was charged, and one that holds a reservation. */
async function prepared() {
  const rider = await newAccount(`${ACCOUNT_PREFIX}-rider`);
  const holder = await newAccount(`${ACCOUNT_PREFIX}-holder`);

  const created = await reserve(await availableVehicle(), newCommandKey(), rider);
  assert.equal(created.status, 201, created.text);
  const rentalId = created.json.rental.id;
  const started = await rideCommand(`/api/v1/reservations/${rentalId}/start`, rider);
  assert.equal(started.status, STATUS_OK, started.text);
  const finished = await rideCommand(`/api/v1/rides/${rentalId}/finish`, rider);
  assert.equal(finished.status, STATUS_OK, finished.text);
  const invoiceId = sql(`SELECT id FROM invoices WHERE rental_id = '${rentalId}'`);
  assert.ok(invoiceId, 'the finished ride was not invoiced');

  const held = await reserve(await availableVehicle(), newCommandKey(), holder);
  assert.equal(held.status, 201, held.text);

  return { rider, holder, rentalId, invoiceId, heldRentalId: held.json.rental.id };
}

/**
 * Everything one ride holds, read as one value: the stage it stands in, the invoice it was charged by,
 * the notifications about it, the unfinished tasks it owes and the results saved for its account.
 * There is no line table — an invoice's lines are derived from the one invoice row on every read — so
 * "no line was written" is stated by the invoice count itself. A failure inside the ending must leave
 * every part of this exactly as it was.
 */
function stateOfRide(userId, rentalId) {
  return JSON.parse(
    sql(
      `SELECT json_build_object(
         'stage', (SELECT stage FROM rentals WHERE id = '${rentalId}'),
         'invoices', (SELECT count(*) FROM invoices WHERE rental_id = '${rentalId}'),
         'notifications', (SELECT count(*) FROM notifications WHERE rental_id = '${rentalId}'),
         'unfinishedTasks', (SELECT count(*) FROM outbox WHERE resource_id = '${rentalId}' AND completed_at IS NULL),
         'savedResults', (SELECT count(*) FROM idempotency_requests WHERE user_id = '${userId}')
       )`,
    ),
  );
}

/** How many unfinished deliveries one ride still owes, which a check waits to fall to none. */
function unfinishedTasksOf(rentalId) {
  return count(`SELECT count(*) FROM outbox WHERE resource_id = '${rentalId}' AND completed_at IS NULL`);
}

function rideCommand(path, account) {
  return call(path, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
  });
}

function count(query) {
  return Number(sql(query));
}

function accountId(account) {
  const id = sql(`SELECT id FROM users WHERE email = '${account.email}'`);
  assert.ok(id, `the account ${account.email} was never registered`);
  return id;
}

/**
 * The moment the payment row of one invoice states, in the form the contract publishes: the database
 * renders a moment in its own way, and what is compared with an answer is the moment, not the
 * rendering. It is null when the row states none.
 */
function storedPaidAt(invoiceId) {
  const moment = sql(
    `SELECT to_char(paid_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
     FROM invoice_payments WHERE invoice_id = '${invoiceId}'`,
  );
  return moment === '' ? null : moment;
}

function storedTotal(invoiceId) {
  return sql(`SELECT total_amount_tyiyn FROM invoices WHERE id = '${invoiceId}'`);
}

/** Removes the rows this suite wrote, so the prepared demonstration can be put back. */
function endSuite() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(`DELETE FROM outbox WHERE recipient_id IN ${mine}`);
  sql(`DELETE FROM outbox WHERE resource_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM invoice_payments WHERE invoice_id IN (SELECT id FROM invoices WHERE user_id IN ${mine})`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
  sql(`DELETE FROM ride_segments WHERE rental_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  sql(`DELETE FROM users WHERE id IN ${mine}`);
}
