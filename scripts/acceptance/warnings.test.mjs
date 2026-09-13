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

/** The zone every stored moment is rendered in, which is the one the contract publishes. */
const STORED_MOMENT_ZONE = 'UTC';

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

describe('the last minute of a reservation', () => {
  test('warns a reservation inside its last minute exactly once', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-inside`);
    const vehicleId = await availableVehicle();
    const rentalId = prepareReservation({ account, vehicleId, deadlineSeconds: 30 });

    await until(() => notificationsOf(rentalId).length === 1, 'no warning was created for the last minute');

    const [warning] = notificationsOf(rentalId);
    assert.equal(warning.kind, 'reservation_expiring');
    assert.equal(warning.active, true);
    assert.equal(warning.version, 1);
    assert.equal(warning.readAt, '');
    assert.equal(warning.userId, await accountId(account.email));

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

  test('does not warn a reservation that has more than its last minute left', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-early`);
    const vehicleId = await availableVehicle();
    const rentalId = prepareReservation({ account, vehicleId, deadlineSeconds: 300 });

    await delay(QUIET_MS);

    assert.deepEqual(notificationsOf(rentalId), [], 'a warning was created before the last minute');
    assert.equal(signalsOf(rentalId).length, 0, 'a signal was queued before the last minute');
    assert.equal(storedRental(rentalId)[0], 'reserved');
  });

  test('releases a reservation whose deadline has passed instead of warning it', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-past`);
    const vehicleId = await availableVehicle();
    const rentalId = prepareReservation({ account, vehicleId, deadlineSeconds: -1 });

    await until(() => storedRental(rentalId)[0] === 'expired', 'the pass did not release the reservation');

    const stored = storedRental(rentalId);
    assert.equal(stored[6], stored[5], `the reservation ended at ${stored[6]} rather than its deadline`);
    assert.deepEqual(notificationsOf(rentalId), [], 'a warning was created after the deadline');
    assert.equal(signalsOf(rentalId).length, 0, 'a signal was queued after the deadline');
    assert.equal((await publishedVehicle(vehicleId)).status, 'available');
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
    const account = await newAccount(`${ACCOUNT_PREFIX}-failing`);
    const vehicleId = await availableVehicle();
    const rentalId = prepareReservation({ account, vehicleId, deadlineSeconds: 30 });

    sql('REVOKE SELECT, INSERT ON notifications FROM carsharing_app');
    try {
      await delay(QUIET_MS);
      const logs = compose('logs', '--tail', '80', 'worker');
      assert.match(logs, /"work":"reservation deadlines"/, 'a failing deadline pass was not reported');
      assert.deepEqual(notificationsOf(rentalId), [], 'a warning was created by a pass that failed');
    } finally {
      sql('GRANT SELECT, INSERT ON notifications TO carsharing_app');
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
      dateReservation(rentalId, "(now() AT TIME ZONE 'Asia/Bishkek')::date - interval '1 day'");
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
 */
function prepareReservation({
  account,
  vehicleId,
  deadlineSeconds,
  startedSecondsAgo = RESERVATION_STARTED_SECONDS_AGO,
}) {
  const id = randomUUID();
  sql(
    insertRental({
      id,
      email: account.email,
      vehicleId,
      stage: 'reserved',
      reservedAt: `clock_timestamp() - make_interval(secs => ${startedSecondsAgo})`,
      expiresAt: `clock_timestamp() + make_interval(secs => ${deadlineSeconds})`,
    }),
  );
  return id;
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
