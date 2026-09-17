// The reservation lifecycle, observed on the real HTTP boundary and in the PostgreSQL the running
// service reads: the races, the boundaries that decide an answer, the read that records an expiry,
// and the repeat that must answer what the first attempt answered.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import { setTimeout as delay } from 'node:timers/promises';
import { call, signInFromSecondDevice, sql, waitForReady } from './client.mjs';
import { holdTransaction } from './fleet.mjs';
import {
  accountId,
  availableVehicle,
  availableVehicles,
  cancel,
  compose,
  currentOf,
  dateReservation,
  endSuiteReservations,
  liveRentals,
  moveDeadline,
  newAccount,
  newCommandKey,
  outboxTasksAddressedTo,
  outboxTasksFor,
  publishedVehicle,
  race,
  reserve,
  reservationsOnDay,
  storedRental,
  storedResults,
  vehicleWithStatus,
} from './reservations.mjs';

const DAY_LIMIT_CODE = 'DAILY_LIMIT_REACHED';
const FIFTEEN_MINUTES_MS = 900_000;
const MOMENT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;

before(async () => {
  await waitForReady();
});

// The demonstration publishes a handful of free vehicles, and every check below takes one: what a
// check left holding is given back before the next one starts, so a suite does not run out of fleet.
beforeEach(async () => {
  await endSuiteReservations();
});

// Every reservation this suite made is given back before it finishes, so the suites that follow read
// the prepared demonstration rather than one a check left behind.
after(async () => {
  await endSuiteReservations();
});

describe('taking one vehicle for fifteen minutes', () => {
  test('answers 201 with the stored conditions and the exact deadline', async () => {
    const account = await newAccount('reserve-basic');
    const vehicleId = await availableVehicle();

    const answer = await reserve(vehicleId, newCommandKey(), account);
    assert.equal(answer.status, 201, answer.text);

    const rental = answer.json.rental;
    assert.match(rental.reserved_at, MOMENT);
    assert.equal(rental.state, 'reserved');
    assert.equal(Date.parse(rental.expires_at) - Date.parse(rental.reserved_at), FIFTEEN_MINUTES_MS);

    const stored = storedRental(rental.id);
    assert.equal(stored[0], 'reserved');
    assert.equal(stored[9], '00:15:00', `the stored interval is ${stored[9]}`);

    // The conditions are the ones the price list held at that moment: they travel with the rental
    // rather than being read from the catalog when the answer is written.
    const offered = await offeredTariff();
    assert.equal(rental.tariff_snapshot.driving_rate_tyiyn_per_started_minute, offered.driving);
    assert.equal(rental.tariff_snapshot.paused_rate_tyiyn_per_started_minute, offered.paused);
    assert.equal(stored[7], offered.driving);
    assert.equal(stored[8], offered.paused);

    const current = await currentOf(account);
    assert.equal(current.status, 200, current.text);
    assert.equal(current.json.kind, 'rental');
    assert.equal(current.json.rental.id, rental.id);
    assert.equal(current.json.daily_limit.available, false);
  });

  test('publishes the vehicle as reserved on the public map', async () => {
    const account = await newAccount('reserve-map');
    const vehicleId = await availableVehicle();
    const answer = await reserve(vehicleId, newCommandKey(), account);
    assert.equal(answer.status, 201, answer.text);

    assert.equal((await publishedVehicle(vehicleId)).status, 'reserved');
  });

  test('refuses a vehicle that is not fit to start, with the catalog reasons', async () => {
    const account = await newAccount('reserve-unfit');
    const unfit = await vehicleWithStatus('unavailable');

    const answer = await reserve(unfit.id, newCommandKey(), account);
    assert.equal(answer.status, 409, answer.text);
    assert.equal(answer.json.code, 'VEHICLE_UNAVAILABLE');
    assert.ok(answer.json.details.unavailable_reasons.length > 0, answer.text);

    // Nothing was taken and nothing was spent: the account may still reserve today.
    assert.equal(liveRentals('user_id', await accountId(account.email)), 0);
    const current = await currentOf(account);
    assert.equal(current.json.kind, 'none');
    assert.equal(current.json.daily_limit.available, true);
  });

  test('refuses a vehicle that does not exist as an unavailable one', async () => {
    const account = await newAccount('reserve-absent');
    const answer = await reserve('01994342-6ba7-7000-8000-000900000999', newCommandKey(), account);
    assert.equal(answer.status, 409, answer.text);
    assert.equal(answer.json.code, 'VEHICLE_UNAVAILABLE');
    assert.equal(liveRentals('user_id', await accountId(account.email)), 0);
  });

  test('refuses a vehicle another account already holds', async () => {
    const holder = await newAccount('reserve-holder');
    const other = await newAccount('reserve-other');
    const vehicleId = await availableVehicle();
    const taken = await reserve(vehicleId, newCommandKey(), holder);
    assert.equal(taken.status, 201, taken.text);

    const answer = await reserve(vehicleId, newCommandKey(), other);
    assert.equal(answer.status, 409, answer.text);
    assert.equal(answer.json.code, 'VEHICLE_UNAVAILABLE');
    assert.equal(liveRentals('vehicle_id', vehicleId), 1);

    // The account that lost keeps its own day's allowance.
    assert.equal((await currentOf(other)).json.daily_limit.available, true);
  });
});

describe('two accounts racing one vehicle', () => {
  test('produces exactly one reservation, and the loser keeps the day', async () => {
    const first = await newAccount('race-one-a');
    const second = await newAccount('race-one-b');
    const vehicleId = await availableVehicle();

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

    const winner = answers[0].status === 201 ? first : second;
    const loser = answers[0].status === 201 ? second : first;
    assert.equal(liveRentals('user_id', await accountId(winner.email)), 1);
    assert.equal(liveRentals('user_id', await accountId(loser.email)), 0);
    assert.equal((await currentOf(loser)).json.daily_limit.available, true);
  });
});

describe('one account racing two vehicles', () => {
  test('produces at most one reservation and spends the day once', async () => {
    const account = await newAccount('race-two');
    const [firstVehicle, secondVehicle] = await availableVehicles(2);

    const answers = await race([
      () => reserve(firstVehicle, newCommandKey(), account),
      () => reserve(secondVehicle, newCommandKey(), account),
    ]);

    assert.equal(answers.filter((answer) => answer.status === 201).length, 1, answers[0].text);
    assert.equal(liveRentals('user_id', await accountId(account.email)), 1);

    const refused = answers.find((answer) => answer.status !== 201);
    assert.equal(refused.status, 409, refused.text);
    assert.ok(
      ['ACTIVE_RENTAL_EXISTS', DAY_LIMIT_CODE].includes(refused.json.code),
      `the refusal was ${refused.json.code}`,
    );
    assert.equal(reservationsOnDay(await accountId(account.email)), 1);
  });
});

describe('the day of free reservations', () => {
  test('refuses a second reservation with the moment the allowance returns', async () => {
    const account = await newAccount('limit-second');
    const [firstVehicle, secondVehicle] = await availableVehicles(2);
    const first = await reserve(firstVehicle, newCommandKey(), account);
    assert.equal(first.status, 201, first.text);

    const answer = await reserve(secondVehicle, newCommandKey(), account);
    assert.equal(answer.status, 409, answer.text);
    assert.equal(answer.json.code, DAY_LIMIT_CODE);
    assert.equal(answer.json.details.daily_limit.available, false);
    assert.match(answer.json.details.daily_limit.resets_at, MOMENT);
  });

  test('does not return the allowance when the reservation is given back', async () => {
    const account = await newAccount('limit-cancel');
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);

    const cancelled = await cancel(created.json.rental.id, newCommandKey(), account);
    assert.equal(cancelled.status, 200, cancelled.text);

    const answer = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(answer.status, 409, answer.text);
    assert.equal(answer.json.code, DAY_LIMIT_CODE);
  });

  test('spends only the day the reservation was made in', async () => {
    const account = await newAccount('limit-yesterday');
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);
    const cancelled = await cancel(created.json.rental.id, newCommandKey(), account);
    assert.equal(cancelled.status, 200, cancelled.text);

    // Dated to the day before, which is the day it spent: a reservation made before local midnight
    // spends the day that is ending where the service is, not the day a later request arrives in.
    dateReservation(created.json.rental.id, "(now() AT TIME ZONE 'Asia/Bishkek')::date - interval '1 day'");

    const answer = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(answer.status, 201, answer.text);
  });

  test('keeps the live reservation of yesterday blocking a new one', async () => {
    const account = await newAccount('limit-live-yesterday');
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);
    dateReservation(created.json.rental.id, "(now() AT TIME ZONE 'Asia/Bishkek')::date - interval '1 day'");

    const answer = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(answer.status, 409, answer.text);
    assert.equal(answer.json.code, 'ACTIVE_RENTAL_EXISTS');
  });

  test('decides the day from the moment read after a wait for a lock', async () => {
    const account = await newAccount('limit-lock-wait');
    const userId = await accountId(account.email);
    const vehicleId = await availableVehicle();

    const before = Date.now();
    const holding = holdTransaction([`SELECT id FROM users WHERE id = '${userId}' FOR UPDATE;`], 6);
    // The holding connection has to reach its statement before the command is sent, or the command
    // would find the row free and the check would prove nothing.
    await delay(3_000);
    const answer = await reserve(vehicleId, newCommandKey(), account);
    const waited = Date.now() - before;
    await holding;

    assert.equal(answer.status, 201, answer.text);
    assert.ok(waited >= 1_000, `the command did not wait for the lock: ${waited} ms`);
    // The reserved moment is the one the command fixed after the wait rather than the one the
    // request arrived at, so it cannot precede the lock the command waited for.
    const reservedAt = Date.parse(answer.json.rental.reserved_at);
    assert.ok(reservedAt - before >= 1_000, `reserved_at ${answer.json.rental.reserved_at} predates the wait`);
  });
});

describe('giving a reservation back', () => {
  test('frees the vehicle for another account with an unspent day', async () => {
    const owner = await newAccount('cancel-owner');
    const other = await newAccount('cancel-other');
    const vehicleId = await availableVehicle();
    const created = await reserve(vehicleId, newCommandKey(), owner);
    assert.equal(created.status, 201, created.text);

    const cancelled = await cancel(created.json.rental.id, newCommandKey(), owner);
    assert.equal(cancelled.status, 200, cancelled.text);
    assert.equal(cancelled.json.rental.state, 'cancelled');

    const stored = storedRental(created.json.rental.id);
    assert.equal(stored[0], 'cancelled');
    assert.equal(liveRentals('vehicle_id', vehicleId), 0);
    assert.equal((await publishedVehicle(vehicleId)).status, 'available');

    const taken = await reserve(vehicleId, newCommandKey(), other);
    assert.equal(taken.status, 201, taken.text);
  });

  test('cannot give back another account reservation, or discover that it exists', async () => {
    const owner = await newAccount('foreign-owner');
    const stranger = await newAccount('foreign-stranger');
    const created = await reserve(await availableVehicle(), newCommandKey(), owner);
    assert.equal(created.status, 201, created.text);

    const foreign = await cancel(created.json.rental.id, newCommandKey(), stranger);
    const unknown = await cancel('01994342-6ba7-7000-8000-000900000999', newCommandKey(), stranger);
    assert.equal(foreign.status, 404, foreign.text);
    assert.equal(unknown.status, 404, unknown.text);
    assert.equal(foreign.json.code, unknown.json.code);
    assert.equal(storedRental(created.json.rental.id)[0], 'reserved');
  });

  test('records the expiry and answers expired when the deadline has passed', async () => {
    const account = await newAccount('cancel-late');
    const vehicleId = await availableVehicle();
    const created = await reserve(vehicleId, newCommandKey(), account);
    assert.equal(created.status, 201, created.text);

    moveDeadline(created.json.rental.id, -1);
    const answer = await cancel(created.json.rental.id, newCommandKey(), account);
    assert.equal(answer.status, 409, answer.text);
    assert.equal(answer.json.code, 'RESERVATION_EXPIRED');

    // The refusal did not undo the transition that had become due: the reservation ended at its own
    // deadline, its vehicle is free and both changes were queued.
    const stored = storedRental(created.json.rental.id);
    assert.equal(stored[0], 'expired');
    assert.equal(stored[6], stored[5], `ended_at ${stored[6]} is not the deadline ${stored[5]}`);
    assert.equal(liveRentals('vehicle_id', vehicleId), 0);
    assert.ok(outboxTasksFor(created.json.rental.id) >= 1, 'no signal was queued for the expiry');
    assert.equal((await publishedVehicle(vehicleId)).status, 'available');
  });
});

describe('the read that answers what is current', () => {
  test('answers none with the day allowance when nothing is current', async () => {
    const account = await newAccount('current-none');
    const answer = await currentOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.equal(answer.json.kind, 'none');
    assert.equal(answer.json.daily_limit.available, true);
    assert.match(answer.json.server_time, MOMENT);
  });

  test('records the expiry it discovers and frees the vehicle without the worker', async () => {
    const account = await newAccount('current-expiry');
    const vehicleId = await availableVehicle();
    const created = await reserve(vehicleId, newCommandKey(), account);
    assert.equal(created.status, 201, created.text);

    compose('stop', 'worker');
    try {
      moveDeadline(created.json.rental.id, -1);
      const answer = await currentOf(account);
      assert.equal(answer.status, 200, answer.text);
      assert.equal(answer.json.kind, 'none');
      assert.equal(storedRental(created.json.rental.id)[0], 'expired');
      assert.equal(liveRentals('vehicle_id', vehicleId), 0);
      assert.equal((await publishedVehicle(vehicleId)).status, 'available');
    } finally {
      compose('start', 'worker');
    }
  });

  test('writes nothing when no transition is due', async () => {
    const account = await newAccount('current-quiet');
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);

    const before = Number(storedRental(created.json.rental.id)[1]);
    const answer = await currentOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.equal(answer.json.kind, 'rental');

    // An ordinary read is not a transition: neither the rental nor its vehicle moved.
    assert.equal(Number(storedRental(created.json.rental.id)[1]), before);
    assert.equal(answer.json.rental.version, String(before));
  });
});

describe('repeating a command whose answer was lost', () => {
  test('answers the stored result, marked as a repeat', async () => {
    const account = await newAccount('repeat-lost');
    const vehicleId = await availableVehicle();
    const key = newCommandKey();

    const first = await reserve(vehicleId, key, account);
    assert.equal(first.status, 201, first.text);

    const repeat = await reserve(vehicleId, key, account);
    assert.equal(repeat.status, 201, repeat.text);
    assert.equal(repeat.headers.get('Idempotency-Replayed'), 'true');
    assert.deepEqual(repeat.json, first.json);
    assert.equal(liveRentals('user_id', await accountId(account.email)), 1);
  });

  test('answers a parallel repeat with the one reservation it made', async () => {
    const account = await newAccount('repeat-parallel');
    const vehicleId = await availableVehicle();
    const key = newCommandKey();

    const answers = await race([() => reserve(vehicleId, key, account), () => reserve(vehicleId, key, account)]);

    for (const answer of answers) assert.equal(answer.status, 201, answer.text);
    assert.equal(answers[0].json.rental.id, answers[1].json.rental.id);
    assert.equal(liveRentals('user_id', await accountId(account.email)), 1);
  });

  test('survives a restart of the API', async () => {
    const account = await newAccount('repeat-restart');
    const vehicleId = await availableVehicle();
    const key = newCommandKey();
    const first = await reserve(vehicleId, key, account);
    assert.equal(first.status, 201, first.text);

    compose('restart', 'api');
    await waitForReady();

    const repeat = await reserve(vehicleId, key, account);
    assert.equal(repeat.status, 201, repeat.text);
    assert.equal(repeat.headers.get('Idempotency-Replayed'), 'true');
    assert.deepEqual(repeat.json, first.json);
  });

  test('refuses the same key applied to another command', async () => {
    const account = await newAccount('repeat-conflict');
    const [firstVehicle, secondVehicle] = await availableVehicles(2);
    const key = newCommandKey();
    const first = await reserve(firstVehicle, key, account);
    assert.equal(first.status, 201, first.text);

    const otherVehicle = await reserve(secondVehicle, key, account);
    assert.equal(otherVehicle.status, 409, otherVehicle.text);
    assert.equal(otherVehicle.json.code, 'IDEMPOTENCY_CONFLICT');

    // The path is part of the fingerprint, so the same key on another operation conflicts too.
    const otherOperation = await cancel(first.json.rental.id, key, account);
    assert.equal(otherOperation.status, 409, otherOperation.text);
    assert.equal(otherOperation.json.code, 'IDEMPOTENCY_CONFLICT');

    const stored = storedRental(first.json.rental.id);
    assert.equal(stored[0], 'reserved');
  });

  test('keeps the result of one account out of another account answers', async () => {
    const owner = await newAccount('repeat-owner');
    const stranger = await newAccount('repeat-stranger');
    const vehicleId = await availableVehicle();
    const key = newCommandKey();
    const first = await reserve(vehicleId, key, owner);
    assert.equal(first.status, 201, first.text);

    const foreign = await reserve(vehicleId, key, stranger);
    assert.ok(!foreign.text.includes(first.json.rental.id), foreign.text);
    assert.equal(liveRentals('vehicle_id', vehicleId), 1);
  });

  test('survives a new session of the same account', async () => {
    const account = await newAccount('repeat-session');
    const vehicleId = await availableVehicle();
    const key = newCommandKey();
    const first = await reserve(vehicleId, key, account);
    assert.equal(first.status, 201, first.text);

    // A second device replaces the session the command was sent with, which must not make the
    // answer of a command that was already made unreachable.
    const secondDevice = await signInFromSecondDevice(account.email);
    const repeat = await reserve(vehicleId, key, secondDevice);
    assert.equal(repeat.status, 201, repeat.text);
    assert.equal(repeat.headers.get('Idempotency-Replayed'), 'true');
    assert.deepEqual(repeat.json, first.json);
  });

  test('keeps a result inside its retention and removes the one past it', async () => {
    const account = await newAccount('repeat-expiry');
    const userId = await accountId(account.email);
    const inside = newCommandKey();
    const past = newCommandKey();

    // Two stored results of one account: one whose retention has passed and one whose retention has
    // not. The worker's sweep is what removes a result, so the rows are written the way a completed
    // command writes them and the check waits for the sweep rather than calling it.
    for (const [key, age] of [
      [inside, '1 hour'],
      [past, '25 hours'],
    ]) {
      sql(
        `INSERT INTO idempotency_requests (owner, user_id, command_key, fingerprint, claimed_at,
                                            status_code, body, completed_at, retain_until)
         VALUES ('account:${userId}', '${userId}', '${key}', 'probe', now() - interval '${age}',
                 200, '{}'::jsonb, now() - interval '${age}',
                 now() - interval '${age}' + interval '24 hours')`,
      );
    }
    assert.equal(storedResults(userId), 2, 'the two results were not written');

    await untilStoredResults(userId, 1);
    const left = sql(`SELECT command_key FROM idempotency_requests WHERE user_id = '${userId}'`);
    assert.equal(left, inside, 'the sweep removed the result inside its retention or kept the expired one');

    // A result the sweep removed is one no command may replay, and the account is not left unable to
    // make a new one: the key it used is free again.
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);
  });

  test('stores a result for at least a day after it was written', async () => {
    const account = await newAccount('repeat-retention');
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);

    const retained = sql(
      `SELECT (retain_until - completed_at) >= interval '24 hours' FROM idempotency_requests ` +
        `WHERE user_id = '${await accountId(account.email)}'`,
    );
    assert.equal(retained, 't');
  });

  test('leaves no result behind for a command the database refused', async () => {
    const account = await newAccount('repeat-rollback');
    const userId = await accountId(account.email);
    const vehicleId = await availableVehicle();

    // A command that cannot write its change must leave neither a result nor a claimed key, so the
    // same command may be made again once the database can carry it out.
    sql('REVOKE INSERT ON rentals FROM carsharing_app');
    try {
      const refused = await reserve(vehicleId, newCommandKey(), account);
      assert.equal(refused.status, 503, refused.text);
    } finally {
      sql('GRANT INSERT ON rentals TO carsharing_app');
    }

    assert.equal(storedResults(userId), 0);
    assert.equal(liveRentals('user_id', userId), 0);
    assert.equal(outboxTasksAddressedTo(userId, 'rental.changed'), 0, 'a rolled back command announced a rental');
    assert.equal((await currentOf(account)).json.daily_limit.available, true);

    const answer = await reserve(vehicleId, newCommandKey(), account);
    assert.equal(answer.status, 201, answer.text);
  });
});

/** Waits for the worker's sweep to leave the stated number of results, or reports what it left. */
async function untilStoredResults(userId, expected) {
  const deadline = Date.now() + RETENTION_PATIENCE_MS;
  for (;;) {
    const stored = storedResults(userId);
    if (stored === expected) return;
    if (Date.now() > deadline) {
      throw new Error(`the sweep left ${stored} results where ${expected} were expected`);
    }
    await delay(RETENTION_POLL_MS);
  }
}

/** How long a check waits for the worker's sweep, and how often it asks again. */
const RETENTION_PATIENCE_MS = 40_000;
const RETENTION_POLL_MS = 500;

/** The price list the catalog offers, as the answer publishes it. */
async function offeredTariff() {
  const answer = await call('/api/v1/tariffs');
  assert.equal(answer.status, 200, answer.text);
  const tariff = answer.json.items[0];
  return {
    driving: tariff.driving_rate_tyiyn_per_started_minute,
    paused: tariff.paused_rate_tyiyn_per_started_minute,
  };
}
