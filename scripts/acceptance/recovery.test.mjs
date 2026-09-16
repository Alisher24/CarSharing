// What survives a restart of the service, which is the half of A25 an HTTP check can observe: a
// rental in force, the history of a ride that ended, the invoice it was charged by, and a delivery
// the queue had not made yet. The browser half — a reload of the cabinet — belongs to the end-to-end
// checks, so the line is confirmed by two witnesses rather than by one.
//
// The suite stops and starts the API and the worker under the other suites, so it must not run beside
// them: the acceptance suites are run one at a time for this reason, and a second reader would see
// this suite's restart as its own service failing.
import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';
import { waitForReady } from './client.mjs';
import { INVOICE_ISSUED_TASK, awaitingLetter, deliveredLetter, forgetSuiteMail, until } from './mail.mjs';
import {
  IDEMPOTENCY_HEADER,
  availableVehicles,
  call,
  compose,
  currentOf,
  endSuiteReservations,
  newAccount,
  newCommandKey,
  reserve,
  restoreScenario,
  sql,
} from './reservations.mjs';
import { invoicesOf, ridesOf } from './history.mjs';

/** The accounts this suite registers, which is also how it recognizes its own rows afterwards. */
const ACCOUNT_PREFIX = 'recovery';

/** The services this check restarts, which is the whole of what an operator restarts. */
const RESTARTED_SERVICES = ['api', 'worker'];

before(async () => {
  await waitForReady();
});

after(async () => {
  compose('start', ...RESTARTED_SERVICES);
  await waitForReady();
  await forgetSuiteMail(ACCOUNT_PREFIX);
  await endSuiteRecovery();
  await restoreScenario();
});

describe('what a restart of the service leaves behind', () => {
  test('a rental in force, the ride that ended, its invoice, and the delivery the queue still owes', async () => {
    const { rider, holder, rentalId, invoiceId, heldRentalId } = await prepared();

    // The queue had not delivered the letter about the invoice, because the worker was stopped
    // before the ride was finished: that is the task this restart must not lose.
    assert.equal(awaitingLetter(invoiceId), 1, 'the queue owed no delivery before the restart');

    compose('restart', ...RESTARTED_SERVICES);
    await waitForReady();

    const current = await currentOf(holder);
    assert.equal(current.status, 200, current.text);
    assert.equal(current.json.rental?.id, heldRentalId, 'the rental in force did not survive the restart');

    const history = await ridesOf(rider);
    assert.equal(history.status, 200, history.text);
    assert.deepEqual(
      history.json.items.map((ride) => ride.id),
      [rentalId],
      'the history did not survive the restart',
    );
    assert.equal(history.json.items[0].invoice_id, invoiceId);

    const invoices = await invoicesOf(rider);
    assert.equal(invoices.status, 200, invoices.text);
    assert.deepEqual(
      invoices.json.items.map((view) => view.invoice.id),
      [invoiceId],
      'the invoice did not survive the restart',
    );

    // The worker that came back with the service is what delivers the task nobody accepted, which is
    // the whole of what an unconfirmed task is for.
    const letter = await deliveredLetter(invoiceId);
    assert.notEqual(letter, undefined, `the letter about ${invoiceId} was never delivered`);
    await until(() => awaitingLetter(invoiceId) === 0, `the queue still owes the ${INVOICE_ISSUED_TASK} task`);
  });
});

/**
 * Everything the restart is asked about, set up before it: one account that rode and was charged,
 * and one that still holds a reservation.
 *
 * Two accounts rather than one, because a positive invoice nothing has settled is a debt, and a debt
 * refuses the next reservation: the account that rode cannot also be the one holding a rental.
 *
 * The worker is stopped for the whole setup, so the letter about the invoice stays in the queue and
 * the first payment attempt is never made. Both are tasks the restart has to leave alone.
 */
async function prepared() {
  compose('stop', 'worker');

  const rider = await newAccount(`${ACCOUNT_PREFIX}-rider`);
  const holder = await newAccount(`${ACCOUNT_PREFIX}-holder`);
  const [ridden, held] = await availableVehicles(2);

  const rentalId = await finishedRide(rider, ridden);
  const invoiceId = sql(`SELECT id FROM invoices WHERE rental_id = '${rentalId}'`);
  assert.notEqual(invoiceId, '', 'the finished ride was not invoiced');

  const reserved = await reserve(held, newCommandKey(), holder);
  assert.equal(reserved.status, 201, reserved.text);

  return { rider, holder, rentalId, invoiceId, heldRentalId: reserved.json.rental.id };
}

/** Takes one vehicle, rides it and ends the ride, which is what leaves a history and an invoice. */
async function finishedRide(account, vehicleId) {
  const created = await reserve(vehicleId, newCommandKey(), account);
  assert.equal(created.status, 201, created.text);
  const rentalId = created.json.rental.id;

  const started = await rideCommand(`/api/v1/reservations/${rentalId}/start`, account);
  assert.equal(started.status, 200, started.text);
  const finished = await rideCommand(`/api/v1/rides/${rentalId}/finish`, account);
  assert.equal(finished.status, 200, finished.text);

  return rentalId;
}

function rideCommand(path, account) {
  return call(path, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
  });
}

/** Removes the rows this suite wrote, so the suites after it read the prepared demonstration. */
async function endSuiteRecovery() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(`DELETE FROM outbox WHERE recipient_id IN ${mine}`);
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
  await endSuiteReservations();
}
