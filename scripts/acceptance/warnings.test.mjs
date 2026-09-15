// The reservation warning and the deadline pass, observed against the assembled stack: the minute
// before the deadline and the deadline itself, the pass the worker runs on its own schedule, what a
// stopped worker leaves to the commands and the reads, and the notification the database holds.
//
// The four boundaries of the rule are checked to the microsecond as pure functions in
// `backend/internal/rentals/deadline_test.go`, which is where a moment can be stated exactly. Here
// the deadlines are prepared relative to the clock of the database — the moment every transaction
// acts on is the database's, never this process's — and each boundary is observed through its
// consequences: a warning appears inside the last minute and never outside it, and a reservation
// whose deadline has passed is released instead of warned.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { after, before, beforeEach, describe, test } from 'node:test';
import { setTimeout as delay } from 'node:timers/promises';
import { compose, waitForReady } from './client.mjs';
import { insertRental } from './rentalrows.mjs';
import {
  accountId,
  availableVehicle,
  availableVehicles,
  cancel,
  currentOf,
  dateReservation,
  endSuiteReservations,
  liveRentals,
  moveDeadline,
  newAccount,
  newCommandKey,
  outboxTasksFor,
  publishedVehicle,
  race,
  reservationsOnDay,
  reserve,
  sql,
  storedRental,
  until,
} from './reservations.mjs';

/** The accounts this suite registers, which is also how its rows are recognized afterwards. */
const ACCOUNT_PREFIX = 'warnings';

/** How long a check waits before it believes that nothing wrote a warning. Several passes fit in it. */
const QUIET_MS = 2500;

/** The two moments a prepared reservation is given, as SQL offsets from the database clock. */
const RESERVATION_STARTED_SECONDS_AGO = 840;

/** The lead a reservation observed before its warning window opens is given. */
const BEFORE_WINDOW_SECONDS = 90;

/** The lead a reservation observed at the moment its warning window opens is given. */
const AT_WINDOW_SECONDS = 60;

/** How long before its deadline a reservation is warned, which is the lead the rule declares. */
const WARNING_LEAD_MS = 60_000;

/** The deadline the check of the window's last moment moves its reservation to. */
const FINAL_SECOND_SECONDS = 10;

/** How far from that deadline the check waits to be before it reads what the moment produced. */
const FINAL_SECOND_REMAINING_MS = 9000;

/** The zone every stored moment is rendered in, which is the one the contract publishes. */
const STORED_MOMENT_ZONE = 'UTC';

/**
 * The zone the service day is counted in, which is the one the day's allowance is judged by. It is
 * the timezone the seed writes into `bootstrap_metadata`, restated here because a check that dates a
 * row of its own has to name it.
 */
const SERVICE_DAY_ZONE = 'Asia/Bishkek';

before(async () => {
  await waitForReady();
});

// A check that left a reservation behind would hold a vehicle the next one needs, and the suites
// after this one read the prepared demonstration.
beforeEach(async () => {
  await endSuiteWarnings();
});

after(async () => {
  await endSuiteWarnings();
  compose('up', '--detach', '--scale', 'worker=1', 'worker');
});

// The four moments of the rule, observed through the consequences the assembled stack produces for a
// reservation that sits on each of them. A check states where the reservation stands relative to the
// deadline it stored, and waits for what the deadline pass does with it; the microseconds of the rule
// itself are decided in `backend/internal/rentals/deadline_test.go`, which is where a moment can be
// stated exactly. The window is half-open — [expires_at − one minute, expires_at) — so the first
// moment below is on the near side of it, the second is the moment it opens, the third is its last
// moment, and the fourth is the deadline itself, at which the reservation is released and the warning
// it was given stops being current.
describe('the four moments of the deadline', () => {
  test('before the last minute nothing is written', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-before-window`);
    const rentalId = prepareReservation({
      account,
      vehicleId: await availableVehicle(),
      deadlineSeconds: BEFORE_WINDOW_SECONDS,
    });

    await delay(QUIET_MS);

    // The absence below is worth what the deadline says it is worth, so the deadline and the moment
    // the check reads it at are both read from the database, and what is left of the deadline says
    // whether the window had opened. More than the minute the warning belongs to means it had not.
    const remainingMs = rentalDeadlineRemainingMs(rentalId);
    assert.ok(
      remainingMs > WARNING_LEAD_MS,
      `the check read the row with ${remainingMs} ms left, which is inside the warning window`,
    );
    assert.deepEqual(notificationsOf(rentalId), [], 'a warning was created before the last minute');
    assert.equal(signalsOf(rentalId).length, 0, 'a signal was queued before the last minute');
    assert.equal(storedRental(rentalId)[0], 'reserved');
  });

  test('at the last minute exactly one warning is created', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-window-open`);
    const rentalId = prepareReservation({
      account,
      vehicleId: await availableVehicle(),
      deadlineSeconds: AT_WINDOW_SECONDS,
    });

    await until(() => notificationsOf(rentalId).length === 1, 'no warning was created once the last minute had begun');

    const [warning] = notificationsOf(rentalId);
    assert.equal(warning.kind, 'reservation_expiring');
    assert.equal(warning.active, true);
    assert.equal(warning.version, 1);
    assert.equal(warning.readAt, '');
    assert.equal(warning.userId, await accountId(account.email));

    // The warning the pass created carries a moment inside the half-open window, so what decided it
    // is the deadline rather than the fact that a pass happened to run.
    const deadlineAt = rentalMoment(rentalId, 'expires_at');
    const createdAt = Date.parse(warning.createdAt);
    assert.ok(createdAt >= deadlineAt - WARNING_LEAD_MS, 'the warning was created before its window opened');
    assert.ok(createdAt < deadlineAt, 'the warning was created at or after the deadline');

    const [signal] = signalsOf(rentalId);
    assert.equal(signal.kind, 'notification.changed');
    assert.equal(signal.version, 1);
    assert.equal(signal.recipient, warning.userId, 'the signal was not addressed to the owner');
    assert.equal(storedRental(rentalId)[0], 'reserved', 'the warning released the reservation');

    // A repeated pass meets a warning that is already stored: it writes nothing and leaves the moment
    // the first one stored alone.
    await delay(QUIET_MS);
    const [again] = notificationsOf(rentalId);
    assert.equal(notificationsOf(rentalId).length, 1, 'a repeated pass created a second warning');
    assert.equal(again.id, warning.id);
    assert.equal(again.createdAt, warning.createdAt, 'a repeated pass replaced the stored moment');
    assert.equal(again.version, 1, 'a repeated pass moved the version');
  });

  test('at the last moment of the window the warning stands and the reservation is still held', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-window-last`);
    const vehicleId = await availableVehicle();
    const rentalId = prepareReservation({
      account,
      vehicleId,
      deadlineSeconds: AT_WINDOW_SECONDS,
    });

    await until(() => notificationsOf(rentalId).length === 1, 'the pass did not warn the reservation');
    const [warning] = notificationsOf(rentalId);

    // The deadline is moved to the last moment this check observes, and the check waits until the
    // reservation stands inside the final second of it. A pass that has already warned found a
    // reservation one moment before its deadline, and a pass after this reading finds it released
    // rather than warned again, which is what the assertions below say.
    moveDeadline(rentalId, FINAL_SECOND_SECONDS);
    await until(
      () => rentalMoment(rentalId, 'expires_at') - Date.now() <= FINAL_SECOND_REMAINING_MS,
      'the deadline never arrived',
    );

    const [stored] = notificationsOf(rentalId);
    assert.equal(notificationsOf(rentalId).length, 1, 'the last moment created a second warning');
    assert.equal(stored.id, warning.id);
    assert.equal(stored.createdAt, warning.createdAt, 'the last moment replaced the stored moment');
    assert.equal(stored.active, true, 'the warning stopped being current before the deadline');
    assert.equal(storedRental(rentalId)[0], 'reserved', 'the reservation ended before its deadline');
    assert.equal((await publishedVehicle(vehicleId)).status, 'reserved');
    assert.deepEqual(
      signalsOf(rentalId).map((one) => one.version),
      [1],
      'the warning was deactivated before the deadline',
    );
  });

  test('at the deadline the reservation is released and its warning stops being current', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-at-deadline`);
    const taker = await newAccount(`${ACCOUNT_PREFIX}-at-deadline-taker`);
    const vehicleId = await availableVehicle();
    const rentalId = prepareReservation({ account, vehicleId, deadlineSeconds: AT_WINDOW_SECONDS });

    // The warning this reservation is given belongs to the minute that follows, so the deadline below
    // is reached with a warning already stored rather than with nothing.
    await until(() => notificationsOf(rentalId).length === 1, 'the pass did not warn the reservation');
    const [warning] = notificationsOf(rentalId);

    moveDeadline(rentalId, -1);
    await until(() => notificationsOf(rentalId)[0].active === false, 'the deadline left the warning current');

    const stored = storedRental(rentalId);
    assert.equal(stored[0], 'expired', 'the deadline did not release the reservation');
    assert.equal(stored[6], stored[5], `the reservation ended at ${stored[6]} rather than its deadline`);
    assert.equal(notificationsOf(rentalId).length, 1, 'the deadline created a second warning');
    assert.equal(notificationsOf(rentalId)[0].version, 2, 'the deadline did not move the version of the warning');
    assert.deepEqual(
      signalsOf(rentalId).map((one) => one.version),
      [1, 2],
      'the deadline did not announce the warning it ended',
    );
    assert.equal(warning.userId, await accountId(account.email));
    assert.equal((await publishedVehicle(vehicleId)).status, 'available');

    // A vehicle the deadline released is free: another account takes it, and the collection of that
    // account holds no warning about the reservation that ended.
    const taken = await reserve(vehicleId, newCommandKey(), taker);
    assert.equal(taken.status, 201, taken.text);
    assert.notEqual(taken.json.rental.id, rentalId, 'the released vehicle was given back to its holder');
    assert.equal(await reservationsOnDay(await accountId(taker.email)), 1);
    assert.deepEqual(notificationsOf(taken.json.rental.id), [], 'a fresh reservation was warned at once');
    assert.equal(storedRental(rentalId)[0], 'expired');
  });
});

describe('one warning whichever process and however many passes', () => {
  test('warns each reservation once while two workers pass together', async () => {
    compose('up', '--detach', '--scale', 'worker=2', 'worker');
    try {
      const prepared = [];
      for (const vehicleId of await availableVehicles(4)) {
        const account = await newAccount(`${ACCOUNT_PREFIX}-scale`);
        prepared.push(prepareReservation({ account, vehicleId, deadlineSeconds: 40 }));
      }

      for (const rentalId of prepared) {
        await until(() => notificationsOf(rentalId).length === 1, `two workers did not warn ${rentalId}`);
      }
      // Both workers keep passing, and the unique key is what keeps the second write out.
      await delay(QUIET_MS);
      for (const rentalId of prepared) {
        const warnings = notificationsOf(rentalId);
        assert.equal(warnings.length, 1, `two workers created ${warnings.length} warnings for ${rentalId}`);
        assert.equal(warnings[0].version, 1);
      }
    } finally {
      compose('up', '--detach', '--scale', 'worker=1', 'worker');
    }
  });

  test('reports a failing pass and keeps its schedule and the other jobs', async () => {
    // The privilege is withdrawn before the reservations exist: a pass arriving in between would
    // warn the first one, and the check would then observe a warning it did not wait for. Only the
    // write is withdrawn, because that is the failure this check is about; the expiry of the same
    // pass still reads its notifications and is expected to keep working.
    sql('REVOKE INSERT ON notifications FROM carsharing_app');
    let rentalId;
    try {
      const account = await newAccount(`${ACCOUNT_PREFIX}-failing`);
      rentalId = prepareReservation({ account, vehicleId: await availableVehicle(), deadlineSeconds: 30 });
      const overdue = await newAccount(`${ACCOUNT_PREFIX}-failing-overdue`);
      const overdueRentalId = prepareReservation({
        account: overdue,
        vehicleId: await availableVehicle(),
        deadlineSeconds: -30,
      });

      await delay(QUIET_MS);

      const logs = compose('logs', '--tail', '80', 'worker');
      assert.match(logs, /"work":"reservation deadlines"/, 'a failing deadline pass was not reported');
      assert.deepEqual(notificationsOf(rentalId), [], 'a warning was created by a pass that failed');
      // The pass kept doing its other work: what it could not warn, it still released.
      await until(
        () => storedRental(overdueRentalId)[0] === 'expired',
        'a failing warning stopped the release of the same pass',
      );
    } finally {
      sql('GRANT INSERT ON notifications TO carsharing_app');
    }

    // The schedule kept its cadence, so the next pass warns the reservation as soon as the database
    // carries the write again.
    await until(() => notificationsOf(rentalId).length === 1, 'the pass did not resume after its failure');
  });
});

describe('a worker that was stopped', () => {
  test('writes nothing, and the current read creates the due warning once', async () => {
    compose('stop', 'worker');
    try {
      const account = await newAccount(`${ACCOUNT_PREFIX}-read`);
      const vehicleId = await availableVehicle();
      const rentalId = prepareReservation({ account, vehicleId, deadlineSeconds: 30 });

      await delay(QUIET_MS);
      assert.deepEqual(notificationsOf(rentalId), [], 'a warning was written while no worker ran');
      assert.equal(signalsOf(rentalId).length, 0, 'a signal was queued while no worker ran');

      // A read of another account fixes what that account holds and nothing else.
      const stranger = await newAccount(`${ACCOUNT_PREFIX}-stranger`);
      assert.equal((await currentOf(stranger)).status, 200);
      assert.deepEqual(notificationsOf(rentalId), [], 'a read warned a reservation its reader does not hold');

      // Two reads of the same account race for the same due warning. Both reach the transition, and
      // the unique key on the rental and the kind is what leaves one record: a second creation is
      // answered with the stored notification rather than writing another.
      const reads = await race([() => currentOf(account), () => currentOf(account)]);
      for (const read of reads) {
        assert.equal(read.status, 200, read.text);
        assert.equal(read.json.kind, 'rental');
      }

      const [warning] = notificationsOf(rentalId);
      assert.equal(notificationsOf(rentalId).length, 1, 'two racing reads created two warnings');
      assert.equal(warning.kind, 'reservation_expiring');
      assert.equal(warning.active, true);
      assert.equal(warning.version, 1);
      assert.equal(warning.userId, await accountId(account.email));

      // The signal is undelivered because no worker was running to deliver it, which is what makes
      // it the record of the read's own transaction rather than of a pass.
      assert.equal(signalsOf(rentalId).length, 1, 'two racing reads queued two signals');
      const [signal] = signalsOf(rentalId);
      assert.equal(signal.recipient, warning.userId);
      assert.equal(signal.version, 1);
      assert.equal(signal.completedAt, '', 'a signal recorded by the read was already delivered');

      assert.equal((await currentOf(account)).json.kind, 'rental');
      const [unchanged] = notificationsOf(rentalId);
      assert.equal(notificationsOf(rentalId).length, 1, 'a repeated read created a second warning');
      assert.equal(unchanged.createdAt, warning.createdAt, 'a repeated read replaced the stored moment');
      assert.equal(unchanged.version, 1, 'a repeated read moved the version');
    } finally {
      compose('start', 'worker');
    }
  });

  test('creates the missed warning once when the worker returns while the reservation stands', async () => {
    compose('stop', 'worker');
    let rentalId;
    try {
      const account = await newAccount(`${ACCOUNT_PREFIX}-downtime`);
      rentalId = prepareReservation({ account, vehicleId: await availableVehicle(), deadlineSeconds: 40 });

      await delay(QUIET_MS);
      assert.deepEqual(notificationsOf(rentalId), [], 'a warning was written while no worker ran');
    } finally {
      compose('start', 'worker');
    }

    await until(() => notificationsOf(rentalId).length === 1, 'the returned worker created no warning');
    assert.equal(storedRental(rentalId)[0], 'reserved');
    assert.equal(notificationsOf(rentalId)[0].version, 1);
  });

  test('creates nothing at all when the worker returns after the deadline', async () => {
    compose('stop', 'worker');
    let rentalId;
    try {
      const account = await newAccount(`${ACCOUNT_PREFIX}-belated`);
      rentalId = prepareReservation({ account, vehicleId: await availableVehicle(), deadlineSeconds: -30 });

      await delay(QUIET_MS);
      assert.deepEqual(notificationsOf(rentalId), []);

      compose('start', 'worker');
      await until(() => storedRental(rentalId)[0] === 'expired', 'the returned worker released nothing');
      await delay(QUIET_MS);
      assert.deepEqual(notificationsOf(rentalId), [], 'a belated warning was created');
      assert.equal(signalsOf(rentalId).length, 0, 'a belated signal was queued');
    } finally {
      compose('start', 'worker');
    }
  });

  test('creates nothing for a reservation cancelled inside its last minute', async () => {
    compose('stop', 'worker');
    let rentalId;
    try {
      const account = await newAccount(`${ACCOUNT_PREFIX}-cancelled`);
      const created = await reserve(await availableVehicle(), newCommandKey(), account);
      assert.equal(created.status, 201, created.text);
      rentalId = created.json.rental.id;
      moveDeadline(rentalId, 30);

      const cancelled = await cancel(rentalId, newCommandKey(), account);
      assert.equal(cancelled.status, 200, cancelled.text);
      assert.deepEqual(notificationsOf(rentalId), [], 'a cancellation created a warning');

      compose('start', 'worker');
      await delay(QUIET_MS);
      assert.deepEqual(notificationsOf(rentalId), [], 'the returned worker warned a cancelled reservation');
      assert.equal(signalsOf(rentalId).length, 0);
    } finally {
      compose('start', 'worker');
    }
  });
});

describe('the warning of a reservation that stops standing', () => {
  test('deactivates the warning of a reservation cancelled after the warning', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-cancel-late`);
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);
    const rentalId = created.json.rental.id;
    moveDeadline(rentalId, 20);

    await until(() => notificationsOf(rentalId).length === 1, 'the pass did not warn the reservation');
    const [warning] = notificationsOf(rentalId);

    const cancelled = await cancel(rentalId, newCommandKey(), account);
    assert.equal(cancelled.status, 200, cancelled.text);

    const [stopped] = notificationsOf(rentalId);
    assert.equal(notificationsOf(rentalId).length, 1, 'the cancellation created a second notification');
    assert.equal(stopped.active, false, 'the warning is still current after the cancellation');
    assert.equal(stopped.version, 2, 'the deactivation did not move the version');
    assert.equal(stopped.createdAt, warning.createdAt, 'the deactivation replaced the stored moment');
    assert.equal(stopped.userId, warning.userId);

    const signals = signalsOf(rentalId);
    assert.deepEqual(
      signals.map((one) => one.version),
      [1, 2],
      'the deactivation recorded no signal',
    );
    assert.equal(signals[1].recipient, warning.userId);

    const again = await cancel(rentalId, newCommandKey(), account);
    assert.equal(again.status, 409, again.text);
    assert.equal(again.json.code, 'INVALID_RENTAL_STATE');
    assert.equal(notificationsOf(rentalId)[0].version, 2, 'a repeated cancellation moved the version');
    assert.equal(signalsOf(rentalId).length, 2, 'a repeated cancellation recorded a second signal');
  });

  test('deactivates the warning of a reservation the pass releases', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-expire`);
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);
    const rentalId = created.json.rental.id;
    moveDeadline(rentalId, 20);

    await until(() => notificationsOf(rentalId).length === 1, 'the pass did not warn the reservation');
    const [warning] = notificationsOf(rentalId);

    moveDeadline(rentalId, -1);
    await until(() => storedRental(rentalId)[0] === 'expired', 'the pass did not release the reservation');

    const [released] = notificationsOf(rentalId);
    assert.equal(notificationsOf(rentalId).length, 1);
    assert.equal(released.active, false, 'the warning is still current after the deadline');
    assert.equal(released.version, 2);
    assert.equal(released.createdAt, warning.createdAt);
    assert.deepEqual(
      signalsOf(rentalId).map((one) => one.version),
      [1, 2],
    );
  });

  test('deactivates the warning of a reservation a read expires without the worker', async () => {
    compose('stop', 'worker');
    try {
      const account = await newAccount(`${ACCOUNT_PREFIX}-read-expire`);
      const created = await reserve(await availableVehicle(), newCommandKey(), account);
      assert.equal(created.status, 201, created.text);
      const rentalId = created.json.rental.id;
      moveDeadline(rentalId, 30);

      const warned = await currentOf(account);
      assert.equal(warned.status, 200, warned.text);
      assert.equal(warned.json.kind, 'rental');
      assert.equal(notificationsOf(rentalId).length, 1, 'the read did not create the due warning');
      assert.equal(notificationsOf(rentalId)[0].active, true);

      moveDeadline(rentalId, -1);
      const expired = await currentOf(account);
      assert.equal(expired.status, 200, expired.text);
      assert.equal(expired.json.kind, 'none');

      const stored = storedRental(rentalId);
      assert.equal(stored[0], 'expired', 'the read did not fix the expiry it met');
      assert.equal(stored[6], stored[5]);
      const [released] = notificationsOf(rentalId);
      assert.equal(released.active, false, 'the expiry left the warning current');
      assert.equal(released.version, 2, 'the expiry did not move the version');
      assert.deepEqual(
        signalsOf(rentalId).map((one) => one.version),
        [1, 2],
      );
    } finally {
      compose('start', 'worker');
    }
  });
});

describe('an overdue reservation another command meets', () => {
  test('lets another account take its vehicle and leaves exactly one live rental', async () => {
    compose('stop', 'worker');
    try {
      const holder = await newAccount(`${ACCOUNT_PREFIX}-overdue`);
      const other = await newAccount(`${ACCOUNT_PREFIX}-taker`);
      const vehicleId = await availableVehicle();
      const rentalId = prepareReservation({ account: holder, vehicleId, deadlineSeconds: -60 });
      assert.equal((await publishedVehicle(vehicleId)).status, 'reserved');

      const taken = await reserve(vehicleId, newCommandKey(), other);
      assert.equal(taken.status, 201, taken.text);
      assert.equal(taken.json.rental.vehicle.id, vehicleId);

      assert.equal(storedRental(rentalId)[0], 'expired', 'the release was not committed with the answer');
      assert.equal(liveRentals('vehicle_id', vehicleId), 1, 'the vehicle ended up with several live rentals');
      assert.equal(liveRentals('user_id', await accountId(other.email)), 1);
      assert.equal(liveRentals('user_id', await accountId(holder.email)), 0);
      assert.ok(outboxTasksFor(rentalId) >= 1, 'the release was not announced');
      assert.deepEqual(notificationsOf(rentalId), [], 'an overdue reservation was warned');
    } finally {
      compose('start', 'worker');
    }
  });

  test('produces exactly one reservation when two accounts race the released vehicle', async () => {
    compose('stop', 'worker');
    try {
      const first = await newAccount(`${ACCOUNT_PREFIX}-race-first`);
      const second = await newAccount(`${ACCOUNT_PREFIX}-race-second`);
      const vehicleId = await availableVehicle();
      const rentalId = prepareReservation({
        account: await newAccount(`${ACCOUNT_PREFIX}-race-holder`),
        vehicleId,
        deadlineSeconds: -60,
      });

      const answers = await race([
        () => reserve(vehicleId, newCommandKey(), first),
        () => reserve(vehicleId, newCommandKey(), second),
      ]);

      assert.deepEqual(
        answers.map((answer) => answer.status).sort(),
        [201, 409],
        answers.map((answer) => answer.text).join(' | '),
      );
      assert.equal(liveRentals('vehicle_id', vehicleId), 1);
      assert.equal(storedRental(rentalId)[0], 'expired');
    } finally {
      compose('start', 'worker');
    }
  });

  test('answers its holder from the day allowance rather than from the rental', async () => {
    compose('stop', 'worker');
    try {
      const account = await newAccount(`${ACCOUNT_PREFIX}-holder`);
      const vehicleId = await availableVehicle();
      const rentalId = prepareReservation({ account, vehicleId, deadlineSeconds: -60, startedSecondsAgo: 1200 });

      const refused = await reserve(vehicleId, newCommandKey(), account);
      assert.equal(refused.status, 409, refused.text);
      assert.equal(refused.json.code, 'DAILY_LIMIT_REACHED', refused.text);
      assert.equal(storedRental(rentalId)[0], 'expired', 'the refusal rolled the expiry back');
      assert.equal(liveRentals('vehicle_id', vehicleId), 0, 'the released vehicle is still held');

      // The release stands, so the vehicle is free; dating the spent reservation to the day before
      // leaves today's allowance unspent and the same account takes it again.
      dateReservation(rentalId, `(now() AT TIME ZONE '${SERVICE_DAY_ZONE}')::date - interval '1 day'`);
      const taken = await reserve(vehicleId, newCommandKey(), account);
      assert.equal(taken.status, 201, taken.text);
    } finally {
      compose('start', 'worker');
    }
  });

  test('fixes the expiry when the cancellation arrives after the deadline', async () => {
    compose('stop', 'worker');
    try {
      const account = await newAccount(`${ACCOUNT_PREFIX}-cancel-expired`);
      const vehicleId = await availableVehicle();
      const created = await reserve(vehicleId, newCommandKey(), account);
      assert.equal(created.status, 201, created.text);
      const rentalId = created.json.rental.id;
      moveDeadline(rentalId, -1);

      const answer = await cancel(rentalId, newCommandKey(), account);
      assert.equal(answer.status, 409, answer.text);
      assert.equal(answer.json.code, 'RESERVATION_EXPIRED');

      const stored = storedRental(rentalId);
      assert.equal(stored[0], 'expired');
      assert.equal(stored[6], stored[5], 'the reservation did not end at its own deadline');
      assert.equal(liveRentals('vehicle_id', vehicleId), 0);
      assert.ok(outboxTasksFor(rentalId) >= 1, 'the expiry was not announced');
      assert.deepEqual(notificationsOf(rentalId), [], 'an expiry created a warning');
    } finally {
      compose('start', 'worker');
    }
  });
});

/**
 * Writes one reservation directly, with the deadline a check needs relative to the clock of the
 * database, and returns its identifier. The row is written here rather than through the service
 * because a check has to reach a moment the commands of this build cannot produce on demand; it
 * takes the conditions of the price list in force, so the row says what a reservation of this build
 * says.
 *
 * The moment it was reserved is never earlier than the start of the service day, and the reservation
 * never ends before it begins. The day's allowance counts the reservations of one day, so a check that
 * dated its row "twenty minutes ago" would spend yesterday's allowance whenever it ran in the first
 * minutes of a service day, and a check about the allowance would then meet no refusal at all.
 */
function prepareReservation({
  account,
  vehicleId,
  deadlineSeconds,
  startedSecondsAgo = RESERVATION_STARTED_SECONDS_AGO,
}) {
  const id = randomUUID();
  const reservedAt =
    `greatest(date_trunc('day', clock_timestamp() AT TIME ZONE '${SERVICE_DAY_ZONE}')` +
    ` AT TIME ZONE '${SERVICE_DAY_ZONE}',` +
    ` clock_timestamp() - make_interval(secs => ${startedSecondsAgo}))`;
  sql(
    insertRental({
      id,
      email: account.email,
      vehicleId,
      stage: 'reserved',
      reservedAt,
      expiresAt:
        `greatest(${reservedAt} + interval '1 second',` +
        ` clock_timestamp() + make_interval(secs => ${deadlineSeconds}))`,
    }),
  );
  return id;
}

/**
 * One stored moment of a rental as the epoch milliseconds a check compares with its own clock. The
 * moment is read from the database, because that is the clock every deadline rule acts on.
 */
function rentalMoment(rentalId, column) {
  return Number(sql(`SELECT (extract(epoch FROM ${column}) * 1000)::bigint FROM rentals WHERE id = '${rentalId}'`));
}

/**
 * What is left of one rental's deadline at the moment the database answers, which is the clock the
 * deadline rules act on rather than the clock of this process.
 */
function rentalDeadlineRemainingMs(rentalId) {
  return Number(
    sql(
      `SELECT (extract(epoch FROM expires_at - clock_timestamp()) * 1000)::bigint ` +
        `FROM rentals WHERE id = '${rentalId}'`,
    ),
  );
}

/** Every notification the database holds for one rental, in the order the collection reads them. */
function notificationsOf(rentalId) {
  const rows = sql(
    `SELECT note.id || '|' || note.kind || '|' || note.active || '|' || note.version || '|' ||
            note.user_id || '|' ||
            to_char(note.created_at AT TIME ZONE '${STORED_MOMENT_ZONE}', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || '|' ||
            coalesce(to_char(note.read_at AT TIME ZONE '${STORED_MOMENT_ZONE}', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '')
     FROM notifications note
     WHERE note.rental_id = '${rentalId}'
     ORDER BY note.created_at, note.id`,
  );
  if (rows === '') return [];
  return rows.split('\n').map((row) => {
    const [id, kind, active, version, userId, createdAt, readAt] = row.split('|');
    return { id, kind, active: active === 'true', version: Number(version), userId, createdAt, readAt };
  });
}

/** The personal signals the outbox holds about the notifications of one rental, oldest first. */
function signalsOf(rentalId) {
  const rows = sql(
    `SELECT signal.kind || '|' || signal.recipient_id || '|' || signal.version || '|' ||
            coalesce(signal.completed_at::text, '')
     FROM outbox signal
     JOIN notifications note ON note.id = signal.resource_id
     WHERE note.rental_id = '${rentalId}' AND signal.kind = 'notification.changed'
     ORDER BY signal.version, signal.id`,
  );
  if (rows === '') return [];
  return rows.split('\n').map((row) => {
    const [kind, recipient, version, completedAt] = row.split('|');
    return { kind, recipient, version: Number(version), completedAt };
  });
}

/**
 * Leaves the database as this suite found it. The signal of a warning names the notification rather
 * than the rental, so it is removed before the reservations it belongs to go; the reservations
 * themselves are given back and deleted the way the reservation suites do it.
 */
async function endSuiteWarnings() {
  sql(
    `DELETE FROM outbox WHERE resource_id IN (
       SELECT note.id
       FROM notifications note
       JOIN rentals rental ON rental.id = note.rental_id
       JOIN users account ON account.id = rental.user_id
       WHERE account.email LIKE '${ACCOUNT_PREFIX}-%')`,
  );
  await endSuiteReservations();
}
