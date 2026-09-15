// Paying for a finished ride on the assembled stack: the attempt the service makes by itself, the
// manual repeat after a refusal, the rule that an unsettled invoice keeps the account out of the next
// reservation, and the access the payment operation answers with.
//
// The first attempt belongs to the worker, so a check that must observe an invoice while it is still
// being attempted stops that process and starts it again afterwards. The demand for a decline is
// written as the role that owns the schema, because the application is granted no right to create one:
// that privilege is the whole of the protected demonstration scenario in this build.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import { waitForReady } from './client.mjs';
import {
  ACCOUNT_PREFIX,
  DEMO_FAILED,
  DEMO_PAID,
  INVOICE_CHANGED,
  PAYMENT_ATTEMPT,
  asApplication,
  awaitingAttempt,
  clearPayments,
  debtInvoice,
  demandOf,
  demandOutcome,
  endSuitePayments,
  moveRentalRates,
  owesMoney,
  pay,
  paymentCount,
  prepareIntervals,
  prepareFreeRide,
  restoreRentalRates,
  storedLines,
  storedPayment,
  storedRide,
  storedTotal,
  tasksFor,
} from './payments.mjs';
import {
  IDEMPOTENCY_HEADER,
  availableVehicle,
  call,
  cancel,
  compose,
  liveRentals,
  newAccount,
  newCommandKey,
  race,
  reserve,
  restoreScenario,
  sql,
  until,
} from './reservations.mjs';

/** The moment the contract publishes, which every stored moment is compared against. */
const MOMENT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;

/** The durations the checks give the two intervals of a ride, in seconds. */
const PREPARED_DRIVING_SECONDS = 90;
const PREPARED_PAUSED_SECONDS = 45;

/** The amount the M07 example costs: two begun minutes at 1234 plus one at 321. */
const EXAMPLE_TOTAL_TYIYN = '2789';

/**
 * The version a ride reaches by the time it has ended: it is reserved, started, paused and finished,
 * each of which moves it by one. A payment moves it not at all, which is what the checks compare it at.
 */
const FINISHED_RIDE_VERSION = 4;

/** An amount past the exact range of a double, which a client must receive digit for digit. */
const BEYOND_THE_EXACT_DOUBLE_RANGE = 9_007_199_254_740_993;

before(async () => {
  await waitForReady();
});

// Every check leaves an account, a vehicle and an invoice behind, and the next one needs a free vehicle
// and an account that owes nothing, so what the previous check wrote is removed first.
beforeEach(async () => {
  clearPayments();
});

after(async () => {
  await endSuitePayments();
  restoreScenario();
});

describe('the first attempt at an invoice', () => {
  test('is made by the worker without a command from the client', async () => {
    const { rentalId, invoiceId, vehicleId } = await finishedRide('first-attempt');
    const settled = await settledPayment(invoiceId);
    assert.match(settled.paidAt, MOMENT, 'the settlement states no moment of its own');

    // The view the ending published was version 1; one transition moves it to 2, and the signal of that
    // change was written beside it. The ride itself is untouched: paying it re-opens nothing.
    assert.equal(settled.version, 2, 'the view did not move exactly once');
    // Two changes were announced about this invoice: it was issued, and its payment settled. The one
    // this check is about is the second, which is why the count is exact rather than a floor.
    assert.equal(tasksFor(INVOICE_CHANGED, invoiceId), 2, 'the settlement announced no change or more than one');
    assert.equal(storedRide(rentalId).split('|')[0], 'completed', 'the payment moved the ride it belongs to');
    assert.equal(storedVersion(rentalId), FINISHED_RIDE_VERSION, 'the payment moved the version of the ride');
    assert.equal(storedTotal(invoiceId), EXAMPLE_TOTAL_TYIYN, 'the amount of the settled invoice moved');
    assert.equal(liveRentals('vehicle_id', vehicleId), 0);
  });

  test('settles nothing a second time when the same work is delivered again', async () => {
    const { invoiceId } = await finishedRide('redelivery');
    const settled = await settledPayment(invoiceId);
    // The first task is awaited rather than assumed: the same work written beside a task still being
    // delivered would be one attempt, and this check is about a second one.
    await until(() => awaitingAttempt(invoiceId) === 0, 'the first attempt was never delivered');
    const signals = tasksFor(INVOICE_CHANGED, invoiceId);

    // The same work is recorded a second time, as a task whose lease ran out is recorded again, and the
    // worker delivers it: nothing moves, because a transition applies to an invoice nothing has settled
    // and this one is settled.
    sql(
      `INSERT INTO outbox (kind, resource_id, version, recipient_id)
       SELECT '${PAYMENT_ATTEMPT}', '${invoiceId}', ${settled.version}, user_id
       FROM invoices WHERE id = '${invoiceId}'`,
    );
    await until(() => awaitingAttempt(invoiceId) === 0, 'the task written a second time was never delivered');

    const after = storedPayment(invoiceId);
    assert.equal(after.status, 'paid');
    assert.equal(after.version, 2, 'a repeated delivery moved the version of the view');
    assert.equal(after.paidAt, settled.paidAt, 'a repeated delivery moved the moment of settlement');
    assert.equal(tasksFor(INVOICE_CHANGED, invoiceId), signals, 'a repeated delivery announced a second change');
    assert.equal(paymentCount(invoiceId), 1);
  });
});

describe('an invoice whose first attempt has not been made', () => {
  test('keeps the account in debt and refuses a manual payment', async () => {
    // The ride this check writes is one of today's, so its account has spent the day's allowance on it:
    // a reservation would be refused for that allowance rather than for the debt, which the check below
    // covers with a ride of an earlier day. What is observed here is the debt itself and the payment of
    // an invoice whose attempt the service still owes, with the worker stopped throughout.
    compose('stop', 'worker');
    try {
      const { account, rentalId, invoiceId } = await debtRide('in-flight', { daysAgo: 0 });
      const waiting = storedPayment(invoiceId);
      assert.equal(waiting.status, 'pending', 'the invoice did not stay waiting with the worker stopped');
      assert.equal(waiting.version, 1, 'an invoice waiting for its first attempt already moved its view');
      assert.equal(owesMoney(account), true, 'the invoice waiting for its attempt is not read as a debt');

      // The payment is sent with a key of its own, so the same key reproduces the refusal below rather
      // than paying: a checked refusal is stored with the command that met it.
      const key = newCommandKey();
      const manual = await pay(invoiceId, key, account);
      assert.equal(manual.status, 409, manual.text);
      assert.equal(manual.json.code, 'PAYMENT_IN_PROGRESS', manual.text);
      assert.equal(manual.headers.get('Retry-After'), '1', 'the refusal asks for no wait');
      assert.equal(storedPayment(invoiceId).status, 'pending', 'a manual payment paid an invoice in flight');
      assert.equal(paymentCount(invoiceId), 1);
      assert.equal(storedRide(rentalId).split('|')[0], 'completed');

      const repeat = await pay(invoiceId, key, account);
      assert.equal(repeat.status, 409, repeat.text);
      assert.equal(repeat.json.code, 'PAYMENT_IN_PROGRESS', repeat.text);
      assert.equal(repeat.headers.get('Idempotency-Replayed'), 'true', 'the repeat was not marked as one');
      assert.deepEqual(repeat.json, manual.json, 'the repeat recomputed its answer');
    } finally {
      await startWorker();
    }
  });

  test('is settled by the worker once it runs again, and the account may reserve', async () => {
    compose('stop', 'worker');
    let account;
    let invoiceId;
    try {
      const riding = await finishedRide('resumes');
      account = riding.account;
      invoiceId = riding.invoiceId;
      assert.equal(storedPayment(invoiceId).status, 'pending');
      assert.equal(owesMoney(account), true, 'the account owes money for the invoice of its ride');
    } finally {
      await startWorker();
    }

    await settledPayment(invoiceId);

    // The debt is what kept the account out, and it is the payment that cleared it: the account owes
    // nothing any more, so the rule that refused its reservation no longer applies to it.
    assert.equal(owesMoney(account), false, 'the settled invoice left the account in debt');
  });
});

describe('a refused attempt', () => {
  test('publishes the failure and changes neither the ride nor the invoice', async () => {
    const { rentalId, invoiceId } = await declinedRide('declined');
    const lines = storedLines(invoiceId);
    const total = storedTotal(invoiceId);
    const ride = storedRide(rentalId);

    // The refusal left the invoice as it was priced and the ride as the ending wrote it: an attempt that
    // was turned down pays nothing and un-finishes nothing. The state of the payment is the one the
    // contract publishes for a refusal, with the moment of it and no moment of a settlement.
    const stored = storedPayment(invoiceId);
    assert.equal(stored.status, 'failed');
    assert.equal(stored.failureCode, 'declined');
    assert.equal(stored.version, 2, 'the refusal moved the view of the pending invoice');
    assert.match(stored.failedAt, MOMENT);
    assert.equal(stored.paidAt, '', 'a refused payment states a moment of settlement');

    // Nothing the refusal touched moved: the ride, the amount and the lines are the ones the ending
    // wrote, and the intervals it closed are still closed.
    assert.equal(storedRide(rentalId), ride, 'the refusal moved the ride it belongs to');
    assert.equal(storedTotal(invoiceId), total);
    assert.equal(storedLines(invoiceId), lines);
    assert.equal(openIntervals(rentalId), 0, 'the refusal re-opened an interval of the ride');
    assert.equal(
      sql(`SELECT count(*) FROM outbox WHERE resource_id = '${invoiceId}' AND kind = '${INVOICE_CHANGED}'`),
      '2',
      'a refusal is a change of the invoice and was not announced as one',
    );
  });

  test('keeps the account out of the next reservation until it is paid', async () => {
    // The account of the ride this check writes spent nothing today, so the refusal it meets is the rule
    // under check rather than the day's allowance. The worker is stopped before the debt is written,
    // because the attempt the invoice is owed would otherwise settle it while the check is reading.
    compose('stop', 'worker');
    let account;
    let rentalId;
    let invoiceId;
    try {
      const owed = await debtRide('owed', { daysAgo: 1 });
      account = owed.account;
      rentalId = owed.rentalId;
      invoiceId = owed.invoiceId;
      assert.equal(
        owesMoney(account),
        true,
        `the written invoice is not read as a debt: ${JSON.stringify(storedPayment(invoiceId))}`,
      );
      const blocked = await reserve(await availableVehicle(), newCommandKey(), account);
      assert.equal(blocked.status, 409, blocked.text);
      assert.equal(blocked.json.code, 'OUTSTANDING_INVOICE', blocked.text);
      assert.equal(liveRentals('user_id', ownerId(account)), 0, 'the refusal created a rental');
      assert.equal(owesMoney(account), true, 'the refusal of a debt left the account not in debt');
    } finally {
      await startWorker();
    }

    // The debt is cleared by paying it, and by nothing else: the invoice is settled by the attempt the
    // service owes it, after the demand for success is recorded.
    demandOutcome(rentalId, DEMO_PAID);
    const settled = await settledPayment(invoiceId);
    assert.equal(settled.failureCode, '', 'a settled payment states a reason for a refusal');
    assert.equal(owesMoney(account), false, 'the settled invoice left the account in debt');

    // The same command passes once the invoice is settled, which is what says the debt was the refusal.
    const allowed = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(allowed.status, 201, allowed.text);
  });

  test('is judged before the day allowance, which is what a person clears themselves', async () => {
    // The two conditions are made to stand together, and neither is faked: the day's allowance is spent
    // through the service by a reservation that is given back (giving one back does not return it), and
    // the debt is a positive invoice of an earlier day that nothing has settled. The answer must name
    // the debt, because that is the one of the two a person can do something about.
    compose('stop', 'worker');
    try {
      const account = await newAccount(`${ACCOUNT_PREFIX}-order`);
      const vehicleId = await availableVehicle();
      const created = await reserve(vehicleId, newCommandKey(), account);
      assert.equal(created.status, 201, created.text);
      const cancelled = await cancel(created.json.rental.id, newCommandKey(), account);
      assert.equal(cancelled.status, 200, cancelled.text);

      // The allowance really is spent: the next reservation is refused for it and for nothing else.
      const spent = await reserve(await availableVehicle(), newCommandKey(), account);
      assert.equal(spent.status, 409, spent.text);
      assert.equal(spent.json.code, 'DAILY_LIMIT_REACHED', spent.text);

      // The debt is written for the same account, of a day that does not spend today's allowance, so
      // the only thing the answer can have changed about is which condition is judged first.
      const owed = debtInvoice({ account, vehicleId, daysAgo: 1 });
      assert.equal(owesMoney(account), true, 'the written invoice is not read as a debt');

      const blocked = await reserve(await availableVehicle(), newCommandKey(), account);
      assert.equal(blocked.status, 409, blocked.text);
      assert.equal(blocked.json.code, 'OUTSTANDING_INVOICE', blocked.text);
      assert.equal(liveRentals('user_id', ownerId(account)), 0, 'the refusal created a rental');
      assert.equal(storedPayment(owed.invoiceId).status, 'pending', 'the refusal settled the invoice');
    } finally {
      await startWorker();
    }
  });
});

describe('repeating a payment', () => {
  test('settles a refused invoice exactly once and answers later keys with the view that exists', async () => {
    const { account, invoiceId } = await declinedRide('retry');

    // The refusal is the state a person repeats, and what settles the invoice is an attempt of their
    // own: the transition a manual command makes applies to a refused payment. The task of the refused
    // attempt is awaited, because a worker still holding it would settle the invoice beside this retry.
    await until(() => awaitingAttempt(invoiceId) === 0, 'the refused attempt was never delivered');

    const paid = await pay(invoiceId, newCommandKey(), account);
    assert.equal(paid.status, 200, paid.text);
    assert.equal(paid.json.invoice.payment.status, 'paid', paid.text);
    assert.equal(paid.json.invoice.version, '3', 'the repeat did not move the view exactly once');
    const settled = storedPayment(invoiceId);
    assert.equal(settled.status, 'paid');
    assert.equal(settled.failedAt, '', 'a settled payment still states the moment of a refusal');
    assert.equal(settled.failureCode, '', 'a settled payment still states a reason for a refusal');

    // A new key on a settled invoice answers the view that exists: no attempt, no second moment and no
    // further version.
    const later = await pay(invoiceId, newCommandKey(), account);
    assert.equal(later.status, 200, later.text);
    assert.equal(later.json.invoice.payment.paid_at, paid.json.invoice.payment.paid_at);
    assert.equal(later.json.invoice.version, '3', 'a later key moved the version of the view');
    assert.equal(paymentCount(invoiceId), 1, 'a later key created a second payment');
    assert.equal(storedPayment(invoiceId).paidAt, settled.paidAt);

    // Clearing the command keys does not allow a second payment: nothing about what is owed is stated by
    // a key, and what a new key finds is an invoice somebody has already settled.
    sql(`DELETE FROM idempotency_requests WHERE user_id = '${ownerId(account)}'`);
    const afterClearing = await pay(invoiceId, newCommandKey(), account);
    assert.equal(afterClearing.status, 200, afterClearing.text);
    assert.equal(afterClearing.json.invoice.payment.paid_at, paid.json.invoice.payment.paid_at);
    assert.equal(paymentCount(invoiceId), 1);
  });
});

describe('a ride that cost nothing', () => {
  test('ends with a settled invoice of its own issue moment and no attempt to make', async () => {
    const { rentalId, invoiceId } = await zeroRide('zero');

    assert.equal(storedTotal(invoiceId), '0', 'the check did not finish a ride that cost nothing');
    const stored = storedPayment(invoiceId);
    assert.equal(stored.status, 'paid', 'a zero invoice is waiting for an attempt');
    assert.equal(stored.version, 1, 'a zero invoice moved its view');
    assert.equal(stored.paidAt, issuedAtOf(invoiceId), 'a zero invoice was settled at another moment');
    assert.equal(stored.failedAt, '');
    assert.equal(stored.failureCode, '');

    // No attempt is owed on it, so the queue holds no task of that kind for its invoice, and the answer
    // that ended the ride published the settled state rather than a waiting one.
    assert.equal(tasksFor(PAYMENT_ATTEMPT, invoiceId), 0, 'a zero invoice was given an attempt to make');
    assert.equal(storedVersion(rentalId), FINISHED_RIDE_VERSION);
  });
});

describe('access to paying an invoice', () => {
  test('refuses a session-less request, one without its origin and a foreign invoice', async () => {
    compose('stop', 'worker');
    try {
      const { account, invoiceId } = await finishedRide('access');
      assert.equal(storedPayment(invoiceId).status, 'pending');

      const anonymous = await call(payPathOf(invoiceId), {
        method: 'POST',
        csrfToken: 'a-token-no-session-holds',
        headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
      });
      assert.equal(anonymous.status, 401, anonymous.text);

      const withoutOrigin = await call(payPathOf(invoiceId), {
        method: 'POST',
        cookie: account.cookie,
        csrfToken: account.csrfToken,
        omitOrigin: true,
        headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
      });
      assert.equal(withoutOrigin.status, 403, withoutOrigin.text);
      assert.equal(withoutOrigin.json.code, 'ORIGIN_NOT_ALLOWED', withoutOrigin.text);

      const withoutToken = await call(payPathOf(invoiceId), {
        method: 'POST',
        cookie: account.cookie,
        headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
      });
      assert.equal(withoutToken.status, 403, withoutToken.text);

      const stranger = await newAccount(`${ACCOUNT_PREFIX}-stranger`);
      const foreign = await pay(invoiceId, newCommandKey(), stranger);
      assert.equal(foreign.status, 404, foreign.text);
      assert.equal(foreign.json.code, 'RESOURCE_NOT_FOUND', foreign.text);

      const missing = await pay('01994342-6ba7-7000-8000-0000000000ff', newCommandKey(), account);
      assert.equal(missing.status, 404, missing.text);
      assert.equal(missing.json.code, 'RESOURCE_NOT_FOUND', missing.text);

      // None of the refusals wrote anything: the invoice still waits for the attempt the service owes
      // it, and a stranger's answer about somebody else's invoice says nothing about whether it exists.
      assert.equal(storedPayment(invoiceId).status, 'pending');
      assert.equal(paymentCount(invoiceId), 1);
    } finally {
      await startWorker();
    }
  });
});

describe('a payment in a race with a new reservation', () => {
  test('never lets a reservation through ahead of the payment it depends on', async () => {
    // The two commands one account can send at the same moment: a payment of the invoice it owes for,
    // and the reservation that waits for it. Both lock the account first, so a reservation that passed
    // saw the payment committed — its own moment is not earlier than the moment the invoice was settled.
    const { account, rentalId, invoiceId } = await debtRide('race', { daysAgo: 1 });
    demandOutcome(rentalId, DEMO_PAID);
    const settled = await settledPayment(invoiceId);
    const vehicleId = await availableVehicle();

    const [payment, reservation] = await race([
      () => pay(invoiceId, newCommandKey(), account),
      () => reserve(vehicleId, newCommandKey(), account),
    ]);

    // The invoice is settled, so the payment answers the view that exists rather than paying again, and
    // the reservation is decided by the day's allowance instead of the debt it no longer has.
    assert.equal(payment.status, 200, payment.text);
    assert.equal(payment.json.invoice.payment.status, 'paid', payment.text);
    assert.equal(payment.json.invoice.payment.paid_at, settled.paidAt, 'the race moved the settlement');

    const reservedAt = storedMicroseconds(reservation.json.rental.id, 'rentals', 'id', 'reserved_at');
    const paidAt = storedMicroseconds(invoiceId, 'invoice_payments', 'invoice_id', 'paid_at');
    assert.ok(
      paidAt <= reservedAt,
      `the reservation was made at ${reservedAt}, before the payment at ${paidAt} was committed`,
    );
  });
});

describe('the amount a payment publishes', () => {
  test('keeps every digit of an amount no floating-point number can hold', async () => {
    const { account, invoiceId } = await pricedRide('beyond-2-53');

    // The service owes this invoice an attempt of its own, and it may be delivering that attempt as the
    // check pays: the contract refuses a payment of an invoice whose first attempt is still running, and
    // a refusal is stored against the key that met it. The command is therefore sent again under a key
    // of its own until the service has finished with the invoice, because the key that met the refusal
    // reproduces that refusal rather than paying.
    const settled = await until(async () => {
      const answer = await pay(invoiceId, newCommandKey(), account);
      assert.ok(
        answer.status === 200 || answer.json.code === 'PAYMENT_IN_PROGRESS',
        `paying answered ${answer.status}: ${answer.text}`,
      );
      return answer.status === 200 ? answer : undefined;
    }, `the invoice ${invoiceId} was never settled by the service or paid by the check`);

    // Two begun driving minutes at that rate, which no double holds: read as a number the answer would
    // arrive as a rounded neighbour. The digits are therefore taken from the text of the answer and the
    // product is proven with BigInt.
    const driving = settled.json.invoice.invoice.lines[0];
    const minutes = BigInt(driving.billed_started_minutes);
    const rate = BigInt(driving.rate_tyiyn_per_started_minute);
    assert.equal(rate, BigInt(BEYOND_THE_EXACT_DOUBLE_RANGE), 'the check did not move the rate');
    assert.equal(minutes, 2n, `the check gave the driving mode ${minutes} begun minutes`);

    const expected = minutes * rate + BigInt(settled.json.invoice.invoice.lines[1].amount_tyiyn);
    const published = publishedInteger(settled.text, 'total_amount_tyiyn');
    assert.equal(published, expected.toString(), 'the published digits are not the product');
    assert.ok(
      BigInt(published) > BigInt(Number.MAX_SAFE_INTEGER),
      `the amount ${published} is one a double could hold, so this check proves nothing`,
    );
    assert.notEqual(String(Number(published)), published);
  });
});

describe('the privilege a payment transition runs under', () => {
  test('lets the application move a payment and nothing else', () => {
    const columns = asApplication(
      `UPDATE invoice_payments SET paid_at = paid_at, version = version, updated_at = updated_at,
              status = status, failed_at = failed_at, failure_code = failure_code`,
    );
    assert.equal(columns.accepted, true, `the application cannot move a payment: ${columns.refusal}`);

    for (const [what, statement] of [
      ['the moment a payment was created', `UPDATE invoice_payments SET created_at = created_at`],
      ['the invoice a payment belongs to', `UPDATE invoice_payments SET invoice_id = invoice_id`],
      ['a payment', `DELETE FROM invoice_payments`],
      ['an invoice', `UPDATE invoices SET total_amount_tyiyn = total_amount_tyiyn`],
    ]) {
      const refused = asApplication(statement);
      assert.equal(refused.accepted, false, `the application may write ${what}`);
    }

    const read = asApplication(`SELECT count(*) FROM demo_payment_outcomes`);
    assert.equal(read.accepted, true, `the application cannot read a demand: ${read.refusal}`);
    const spend = asApplication(`DELETE FROM demo_payment_outcomes WHERE false`);
    assert.equal(spend.accepted, true, `the application cannot spend a demand: ${spend.refusal}`);
    const create = asApplication(
      `INSERT INTO demo_payment_outcomes (rental_id, outcome, set_at)
       SELECT id, 'paid', clock_timestamp() FROM rentals WHERE false`,
    );
    assert.equal(create.accepted, false, 'the application may create a demand');
  });
});

/**
 * Registers an account, takes a free vehicle, drives it into the paused stage, gives the ride the
 * durations the checks price and ends it. What comes back is the account, the vehicle, the rental and
 * the invoice of the ride, which is the state every payment check starts from.
 */
async function finishedRide(name, drivingRateTyiyn) {
  const account = await newAccount(`${ACCOUNT_PREFIX}-${name}`);
  const vehicleId = await availableVehicle();
  const created = await reserve(vehicleId, newCommandKey(), account);
  assert.equal(created.status, 201, created.text);
  const rentalId = created.json.rental.id;

  const started = await rideCommand('start', rentalId, account);
  assert.equal(started.status, 200, started.text);
  const paused = await rideCommand('pause', rentalId, account);
  assert.equal(paused.status, 200, paused.text);
  prepareIntervals(rentalId, PREPARED_DRIVING_SECONDS, PREPARED_PAUSED_SECONDS);

  const moved = drivingRateTyiyn === undefined ? undefined : moveRentalRates(rentalId, drivingRateTyiyn, 1);
  try {
    const finished = await finishCommand(rentalId, account);
    assert.equal(finished.status, 200, finished.text);
    return { account, vehicleId, rentalId, invoiceId: finished.json.invoice.invoice.id };
  } finally {
    if (moved !== undefined) restoreRentalRates(rentalId, moved);
  }
}

/** Ends a ride that cost nothing: the price list it was reserved under charges nothing. */
async function zeroRide(name) {
  const account = await newAccount(`${ACCOUNT_PREFIX}-${name}`);
  const vehicleId = await availableVehicle();
  const created = await reserve(vehicleId, newCommandKey(), account);
  assert.equal(created.status, 201, created.text);
  const rentalId = created.json.rental.id;

  const started = await rideCommand('start', rentalId, account);
  assert.equal(started.status, 200, started.text);
  const paused = await rideCommand('pause', rentalId, account);
  assert.equal(paused.status, 200, paused.text);
  const charged = prepareFreeRide(rentalId);

  let finished;
  try {
    finished = await finishCommand(rentalId, account);
  } finally {
    restoreRentalRates(rentalId, charged);
  }
  assert.equal(finished.status, 200, finished.text);
  const invoiceId = finished.json.invoice.invoice.id;
  assert.equal(finished.json.invoice.invoice.total_amount_tyiyn, '0', 'the setup did not end a free ride');
  assert.equal(finished.json.invoice.payment.status, 'paid', 'a zero invoice was published as waiting');
  assert.equal(finished.json.invoice.version, '1');
  return { account, rentalId, invoiceId };
}

/** Ends one ride whose first attempt is refused, with the worker stopped for the refusal. */
async function declinedRide(name) {
  compose('stop', 'worker');
  let riding;
  try {
    riding = await finishedRide(name);
    demandOutcome(riding.rentalId, DEMO_FAILED);
    assert.equal(demandOf(riding.rentalId), DEMO_FAILED, 'the demand was not recorded');
  } finally {
    await startWorker();
  }

  const declined = await until(() => {
    const payment = storedPayment(riding.invoiceId);
    return payment.status === 'failed' ? payment : undefined;
  }, `the attempt at ${riding.invoiceId} was never refused`);

  assert.equal(declined.failureCode, 'declined', 'the refusal states another reason');
  assert.equal(declined.version, 2, 'the refusal moved the view of the pending invoice');
  assert.equal(demandOf(riding.rentalId), '', 'the demand was not spent by the attempt it decided');
  return riding;
}

/** Ends one ride whose driving rate is past the exact range of a double. */
function pricedRide(name) {
  return finishedRide(name, BEYOND_THE_EXACT_DOUBLE_RANGE);
}

/**
 * Registers an account, gives it a finished ride of an earlier day and a positive invoice nothing has
 * settled, and answers the account with the invoice it owes for. The day the ride belongs to is stated
 * by the check, because the free reservation of the current day is what a check about the debt must
 * leave unspent.
 */
async function debtRide(name, { daysAgo }) {
  const account = await newAccount(`${ACCOUNT_PREFIX}-${name}`);
  const vehicleId = await availableVehicle();
  const owed = debtInvoice({ account, vehicleId, daysAgo });
  return { account, vehicleId, ...owed };
}

/** Sends one finish for one rental. */
function finishCommand(rentalId, account) {
  return call(`/api/v1/rides/${rentalId}/finish`, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
  });
}

/** Sends one ride command for one rental, which the setup above uses to reach a ride in progress. */
function rideCommand(operation, rentalId, account) {
  const path = {
    start: `/api/v1/reservations/${rentalId}/start`,
    pause: `/api/v1/rides/${rentalId}/pause`,
  }[operation];
  return call(path, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
  });
}

/** Starts the worker again and waits for the API beside it, which a check stopped it for. */
async function startWorker() {
  compose('start', 'worker');
  await waitForReady();
}

/** Waits until the service settled one invoice, and answers what it stored about the payment. */
function settledPayment(invoiceId) {
  return until(() => {
    const payment = storedPayment(invoiceId);
    return payment.status === 'paid' ? payment : undefined;
  }, `the invoice ${invoiceId} was never settled`);
}

/** The path one payment is sent to. */
function payPathOf(invoiceId) {
  return `/api/v1/me/invoices/${invoiceId}/pay`;
}

/** The identifier of the account behind one address, which no private answer publishes. */
function ownerId(account) {
  return sql(`SELECT id FROM users WHERE email = '${account.email}'`);
}

/** The moment one invoice was issued, rendered the way the contract publishes it. */
function issuedAtOf(invoiceId) {
  return sql(
    `SELECT to_char(issued_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
     FROM invoices WHERE id = '${invoiceId}'`,
  );
}

/** The version one ride reached, which a payment must not move. */
function storedVersion(rentalId) {
  return Number(sql(`SELECT version FROM rentals WHERE id = '${rentalId}'`));
}

/** How many intervals of one ride are still open, of which a finished ride has none. */
function openIntervals(rentalId) {
  return Number(sql(`SELECT count(*) FROM ride_segments WHERE rental_id = '${rentalId}' AND ended_at IS NULL`));
}

/** One stored moment, in the whole microseconds a comparison of two of them needs. */
function storedMicroseconds(id, table, key, column) {
  return Number(sql(`SELECT (extract(epoch FROM ${column}) * 1000000)::bigint FROM ${table} WHERE ${key} = '${id}'`));
}

/**
 * One whole number as the answer published it, read from the text of the body rather than from the
 * parsed one: a value the contract carries as a decimal string is exactly what a check about losing
 * digits has to read, and a parser is the thing that would lose them.
 */
function publishedInteger(body, field) {
  const published = new RegExp(`"${field}":"([0-9]+)"`).exec(body);
  assert.notEqual(published, null, `the answer publishes no ${field}: ${body}`);
  return published[1];
}
