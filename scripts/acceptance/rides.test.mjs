// The ride lifecycle on the assembled stack: the three commands over the real HTTP boundary, the
// intervals they write, what a ride publishes about the time it has taken, and the two refusals a
// start can meet — a reservation whose deadline has passed and a vehicle that cannot begin.
//
// Moments are prepared relative to the clock of the database rather than waited for, as the deadline
// suites do it: a ride with begun minutes in both modes would otherwise cost minutes of wall-clock
// time per check, and a deadline that has passed cannot be waited for at all.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import { setTimeout as delay } from 'node:timers/promises';
import { waitForReady } from './client.mjs';
import {
  IDEMPOTENCY_HEADER,
  availableVehicle,
  call,
  compose,
  currentOf,
  endSuiteReservations,
  liveRentals,
  moveDeadline,
  newAccount,
  newCommandKey,
  publishedVehicle,
  race,
  reserve,
  restoreScenario,
  sql,
  storedRental,
} from './reservations.mjs';

/** The accounts this suite registers, which is also how it recognizes its own rows afterwards. */
const ACCOUNT_PREFIX = 'rides';

/** Where each of the three commands is sent, which is the path its idempotency fingerprint covers. */
const RIDE_PATHS = {
  start: (rentalId) => `/api/v1/reservations/${rentalId}/start`,
  pause: (rentalId) => `/api/v1/rides/${rentalId}/pause`,
  resume: (rentalId) => `/api/v1/rides/${rentalId}/resume`,
};

/** The moment the contract publishes, which every stored moment is compared against. */
const MOMENT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;

/** How long a duration a command answers with may be and still count as one that has just begun. */
const JUST_BEGUN_MICROSECONDS = 5_000_000;

const MICROSECONDS_PER_MINUTE = 60_000_000;

/** How long a check waits for the deadline sweep to have had several passes at the rows it set up. */
const SWEEP_SETTLE_MS = 2_500;

/** The share of its capacity every source of a vehicle is left with, which is below the threshold. */
const DRAINED_BASIS_POINTS = 500;

/** The durations the check of the published progress gives each interval, in seconds. */
const PREPARED_DRIVING_SECONDS = 25;
const PREPARED_PAUSED_SECONDS = 0.25;

/**
 * The durations the check of the specification's example gives the two intervals, in seconds. Ninety
 * seconds of driving have begun two minutes and forty-five seconds of pause have begun one, which is
 * the example M07 states at the demonstration rates of 1234 and 321 tyiyn.
 *
 * The pause is prepared at forty-five seconds and not at the top of its minute, because the interval
 * a paused ride is in is open: the service measures it up to the moment it answers, so the sum is the
 * prepared one plus the second or so the check spends before it reads. Keep this below a minute by
 * more than that delay, or the check measures two begun minutes of pause instead of one.
 */
const EXAMPLE_DRIVING_SECONDS = 90;
const EXAMPLE_PAUSED_SECONDS = 45;

/**
 * The pause the snapshot check prepares. It is shorter than the example's because that check makes
 * three more database round trips than the example does, and a pause it measured past a minute would
 * begin a second minute the amount below does not account for.
 */
const SNAPSHOT_PAUSED_SECONDS = 30;

/** What those durations cost at the demonstration rates of 1234 and 321 tyiyn per begun minute. */
const EXAMPLE_AMOUNT_TYIYN = 2789;

/** The begun minutes ninety seconds of driving are, which every check of an amount states. */
const EXPECTED_DRIVING_STARTED_MINUTES = 2;

/** The rates the demonstration charges, which a reservation stores in its snapshot. */
const DRIVING_RATE_TYIYN = 1234;
const PAUSED_RATE_TYIYN = 321;

/** The rates a check moves the catalog to, standing in for an operator's later price change. */
const MOVED_DRIVING_RATE_TYIYN = 2000;
const MOVED_PAUSED_RATE_TYIYN = 500;

/** An amount past the exact range of a double, which a client must receive digit for digit. */
const BEYOND_THE_EXACT_DOUBLE_RANGE = 9_007_199_254_740_993;

/** How far ahead of the two racing starts the deadline is placed when it must fall between them. */
const ACROSS_SECONDS = 0.25;

before(async () => {
  await waitForReady();
});

// Every check below takes a vehicle, so what one left holding is given back before the next starts.
beforeEach(async () => {
  await endSuiteRides();
});

// The checks move deadlines and energy remainders the prepared scenario owns, so the suite puts the
// demonstration back rather than leaving the next reader a fleet it changed.
after(async () => {
  await endSuiteRides();
  restoreScenario();
});

describe('starting a reservation', () => {
  test('answers 200 with the ride driving, one interval and the moments of one instant', async () => {
    const { account, vehicleId, rentalId } = await reservedRide('start');

    const answer = await rideCommand('start', rentalId, newCommandKey(), account);
    assert.equal(answer.status, 200, answer.text);
    assert.match(answer.json.server_time, MOMENT);

    const rental = answer.json.rental;
    assert.equal(rental.id, rentalId);
    assert.equal(rental.state, 'active');
    assert.equal(rental.started_at, rental.mode_started_at, 'the first mode did not begin when the ride did');
    assert.equal(rental.started_at, storedMoment(rentalId, 'started_at'));
    assert.equal(rental.vehicle.status, 'in_trip');
    assert.equal(rental.vehicle.ride_mode, 'driving');
    assertSmallDuration(rental.progress.driving_duration_microseconds, 'the driving mode that just began');
    assert.equal(rental.progress.paused_duration_microseconds, '0', 'a ride that never paused counted a pause');

    const intervals = assertOneOpenInterval(rentalId, 'after the start');
    assert.deepEqual(
      intervals.map((interval) => interval.mode),
      ['driving'],
      'the start opened more than one interval',
    );
    assert.equal(modeStartMicroseconds(rentalId), intervals[0].startedAt, 'the ride names another interval');
    assert.equal(rental.mode_started_at, storedMoment(rentalId, 'mode_started_at'));

    assert.equal(storedRental(rentalId)[0], 'active');
    assert.equal((await publishedVehicle(vehicleId)).status, 'in_trip');
    assert.equal((await publishedVehicle(vehicleId)).ride_mode, 'driving');
  });
});

describe('pausing a ride', () => {
  test('answers 200 with the ride standing still from the moment the interval changed', async () => {
    const { account, vehicleId, rentalId } = await reservedRide('pause');
    const started = await rideCommand('start', rentalId, newCommandKey(), account);
    assert.equal(started.status, 200, started.text);

    const answer = await rideCommand('pause', rentalId, newCommandKey(), account);
    assert.equal(answer.status, 200, answer.text);

    const rental = answer.json.rental;
    assert.equal(rental.state, 'paused');
    assert.equal(rental.started_at, started.json.rental.started_at, 'pausing moved the moment the ride began');
    assertSmallDuration(rental.progress.paused_duration_microseconds, 'the pause that just began');
    assert.ok(
      Number(rental.progress.driving_duration_microseconds) > 0,
      'the ride that was paused had driven no time at all',
    );
    assert.equal(rental.vehicle.status, 'in_trip');
    assert.equal(rental.vehicle.ride_mode, 'paused');

    const intervals = assertOneOpenInterval(rentalId, 'after the pause');
    assert.deepEqual(
      intervals.map((interval) => interval.mode),
      ['driving', 'paused'],
    );
    assert.equal(intervals[0].endedAt, intervals[1].startedAt, 'the pause began at a moment of its own');
    assert.equal(modeStartMicroseconds(rentalId), intervals[1].startedAt);
    assert.equal(rental.mode_started_at, storedMoment(rentalId, 'mode_started_at'));
    assert.equal((await publishedVehicle(vehicleId)).ride_mode, 'paused');
  });
});

describe('continuing a paused ride', () => {
  test('answers 200 with the ride driving again from the moment the interval changed', async () => {
    const { account, vehicleId, rentalId } = await reservedRide('resume');
    const started = await rideCommand('start', rentalId, newCommandKey(), account);
    assert.equal(started.status, 200, started.text);
    const paused = await rideCommand('pause', rentalId, newCommandKey(), account);
    assert.equal(paused.status, 200, paused.text);

    const answer = await rideCommand('resume', rentalId, newCommandKey(), account);
    assert.equal(answer.status, 200, answer.text);

    const rental = answer.json.rental;
    assert.equal(rental.state, 'active');
    assert.equal(rental.started_at, started.json.rental.started_at, 'continuing moved the moment the ride began');
    assertSmallDuration(rental.progress.driving_duration_microseconds, 'the driving mode that just began');
    assert.equal(rental.vehicle.status, 'in_trip');
    assert.equal(rental.vehicle.ride_mode, 'driving');

    const intervals = assertOneOpenInterval(rentalId, 'after the continuation');
    assert.deepEqual(
      intervals.map((interval) => interval.mode),
      ['driving', 'paused', 'driving'],
    );
    assert.equal(intervals[1].endedAt, intervals[2].startedAt, 'the continuation began at a moment of its own');
    assert.equal(modeStartMicroseconds(rentalId), intervals[2].startedAt);
    assert.equal(rental.mode_started_at, storedMoment(rentalId, 'mode_started_at'));
    assert.equal((await publishedVehicle(vehicleId)).ride_mode, 'driving');
  });
});

describe('the intervals of a mixed ride', () => {
  test('hold one open interval with no gap and no overlap after every transition', async () => {
    const { account, rentalId } = await reservedRide('intervals');
    const sequence = [
      ['start', 'driving'],
      ['pause', 'paused'],
      ['resume', 'driving'],
      ['pause', 'paused'],
    ];

    const modes = [];
    for (const [operation, mode] of sequence) {
      const answer = await rideCommand(operation, rentalId, newCommandKey(), account);
      assert.equal(answer.status, 200, answer.text);

      const intervals = assertOneOpenInterval(rentalId, `after ${operation}`);
      const open = intervals[intervals.length - 1];
      assert.equal(open.mode, mode, `after ${operation} the ride stands in ${open.mode}`);

      // The moment the rental publishes as the start of its current mode names the open interval,
      // both in the database and in the answer that transition produced.
      assert.equal(
        modeStartMicroseconds(rentalId),
        open.startedAt,
        `after ${operation} the ride names no open interval`,
      );
      assert.equal(answer.json.rental.mode_started_at, storedMoment(rentalId, 'mode_started_at'));
      modes.push(open.mode);
    }

    assert.deepEqual(
      modes,
      sequence.map(([, mode]) => mode),
    );
    assert.equal(intervalsOf(rentalId).length, sequence.length, 'a transition wrote more than one interval');
  });
});

describe('repeating a ride command', () => {
  test('answers each of the three commands with the stored body and the replay header', async () => {
    const { account, rentalId } = await reservedRide('repeat-each');

    for (const operation of ['start', 'pause', 'resume']) {
      const key = newCommandKey();
      const first = await rideCommand(operation, rentalId, key, account);
      assert.equal(first.status, 200, first.text);
      const intervals = intervalsOf(rentalId);

      const repeat = await rideCommand(operation, rentalId, key, account);
      assert.equal(repeat.status, 200, `${operation}: ${repeat.text}`);
      assert.equal(repeat.headers.get('Idempotency-Replayed'), 'true', `${operation} was not marked as a repeat`);
      assert.deepEqual(repeat.json, first.json, `${operation} recomputed its answer instead of replaying it`);
      assert.equal(repeat.json.server_time, first.json.server_time, `${operation} replayed another moment`);
      assert.deepEqual(intervalsOf(rentalId), intervals, `${operation} opened a second interval`);
    }
  });

  test('refuses a pause in the paused stage, replays the refusal and opens no interval', async () => {
    const { account, rentalId } = await reservedRide('repeat-stage');
    for (const operation of ['start', 'pause']) {
      const answer = await rideCommand(operation, rentalId, newCommandKey(), account);
      assert.equal(answer.status, 200, answer.text);
    }
    const intervals = intervalsOf(rentalId);

    const key = newCommandKey();
    const refused = await rideCommand('pause', rentalId, key, account);
    assert.equal(refused.status, 409, refused.text);
    assert.equal(refused.json.code, 'INVALID_RENTAL_STATE', refused.text);
    assert.deepEqual(intervalsOf(rentalId), intervals, 'a refused pause opened an interval');
    assert.equal(storedRental(rentalId)[0], 'paused');

    const repeated = await rideCommand('pause', rentalId, key, account);
    assert.equal(repeated.status, 409, repeated.text);
    assert.equal(repeated.headers.get('Idempotency-Replayed'), 'true');
    assert.deepEqual(repeated.json, refused.json, 'the repeat of a refusal answered something else');
    assert.deepEqual(intervalsOf(rentalId), intervals);
  });
});

describe('a start whose reservation deadline has passed', () => {
  test('refuses the start, records the expiry and opens no interval', async () => {
    compose('stop', 'worker');
    try {
      const { account, vehicleId, rentalId } = await reservedRide('overdue');
      moveDeadline(rentalId, -1);

      const answer = await rideCommand('start', rentalId, newCommandKey(), account);
      assert.equal(answer.status, 409, answer.text);
      assert.equal(answer.json.code, 'RESERVATION_EXPIRED', answer.text);

      // The command records the expiry it met rather than only reporting it, and the release is what
      // the answer was committed with.
      const stored = storedRental(rentalId);
      assert.equal(stored[0], 'expired', 'the refused start left the reservation standing');
      assert.equal(stored[6], stored[5], 'the reservation did not end at its own deadline');
      assert.deepEqual(intervalsOf(rentalId), [], 'a refused start opened an interval');
      assert.equal(liveRentals('vehicle_id', vehicleId), 0);
      assert.equal((await publishedVehicle(vehicleId)).status, 'available');
    } finally {
      compose('start', 'worker');
    }
  });

  test('answers one admissible state when two starts race after the deadline', async () => {
    compose('stop', 'worker');
    try {
      const { account, vehicleId, rentalId } = await reservedRide('race-after');
      moveDeadline(rentalId, -1);

      const answers = await race([
        () => rideCommand('start', rentalId, newCommandKey(), account),
        () => rideCommand('start', rentalId, newCommandKey(), account),
      ]);

      for (const answer of answers) {
        assert.equal(answer.status, 409, answer.text);
        assert.equal(answer.json.code, 'RESERVATION_EXPIRED', answer.text);
      }
      assert.deepEqual(intervalsOf(rentalId), [], 'a start that arrived after the deadline opened an interval');
      assert.equal(storedRental(rentalId)[0], 'expired');
      assert.equal(liveRentals('vehicle_id', vehicleId), 0);
    } finally {
      compose('start', 'worker');
    }
  });

  test('begins exactly one ride when two starts race before the deadline', async () => {
    const { account, vehicleId, rentalId } = await reservedRide('race-before');

    const answers = await race([
      () => rideCommand('start', rentalId, newCommandKey(), account),
      () => rideCommand('start', rentalId, newCommandKey(), account),
    ]);

    const begun = answers.filter((answer) => answer.status === 200);
    assert.equal(begun.length, 1, answers.map((answer) => answer.text).join(' | '));
    const refused = answers.find((answer) => answer.status !== 200);
    assert.equal(refused.status, 409, refused.text);
    assert.ok(
      ['INVALID_RENTAL_STATE', 'RESERVATION_EXPIRED'].includes(refused.json.code),
      `the second start was refused with ${refused.json.code}`,
    );

    assert.equal(storedRental(rentalId)[0], 'active');
    const intervals = assertOneOpenInterval(rentalId, 'after the race');
    assert.equal(intervals.length, 1, 'a race opened more than one interval');
    assert.equal(liveRentals('vehicle_id', vehicleId), 1);
  });

  test('leaves exactly one admissible state when the deadline falls inside the race', async () => {
    const { account, vehicleId, rentalId } = await reservedRide('race-across');
    moveDeadline(rentalId, ACROSS_SECONDS);

    const answers = await race([
      () => rideCommand('start', rentalId, newCommandKey(), account),
      () => rideCommand('start', rentalId, newCommandKey(), account),
    ]);
    for (const answer of answers) {
      assert.ok([200, 409].includes(answer.status), answer.text);
    }

    // Which side of the deadline each request landed on is the database's to decide; what the race
    // must never produce is both outcomes at once, or a ride with an interval it does not hold.
    const begun = answers.filter((answer) => answer.status === 200);
    assert.ok(begun.length <= 1, 'both racing starts began a ride');
    const stage = storedRental(rentalId)[0];
    const intervals = intervalsOf(rentalId);
    if (begun.length === 1) {
      assert.equal(stage, 'active', `a start answered 200 and the rental stands ${stage}`);
      assertOneOpenInterval(rentalId, 'after the race');
      assert.equal(intervals.length, 1, 'a race opened more than one interval');
      assert.equal(liveRentals('vehicle_id', vehicleId), 1);
    } else {
      assert.equal(stage, 'expired', `the race left the rental ${stage}`);
      assert.deepEqual(intervals, [], 'an expired reservation kept an interval');
      assert.equal(liveRentals('vehicle_id', vehicleId), 0);
    }
    console.log(`the race across the deadline left the rental ${stage} with ${intervals.length} intervals`);
  });
});

describe('what a ride publishes about the time it has taken', () => {
  test('matches the intervals the database holds, each mode rounded up once', async () => {
    const { account, rentalId } = await reservedRide('progress');
    for (const operation of ['start', 'pause', 'resume', 'pause', 'resume', 'pause']) {
      const answer = await rideCommand(operation, rentalId, newCommandKey(), account);
      assert.equal(answer.status, 200, answer.text);
    }

    // Three driving intervals and three paused ones, the last of them still open. Each driving
    // interval is given less than a minute, so the sum of the mode crosses the minute rather than any
    // one interval: rounding every interval separately would answer three minutes instead of two.
    prepareIntervals(rentalId);
    const intervals = assertOneOpenInterval(rentalId, 'after the prepared durations');
    assert.equal(intervals.filter((interval) => interval.mode === 'driving').length, 3);

    const current = await currentOf(account);
    assert.equal(current.status, 200, current.text);
    assert.equal(current.json.kind, 'rental');
    const rental = current.json.rental;
    assert.equal(rental.id, rentalId);
    assert.equal(rental.state, 'paused');
    assert.equal(rental.mode_started_at, storedMoment(rentalId, 'mode_started_at'));

    // The sums are computed from the intervals the database holds at exactly the moment the answer
    // states, which is the moment the service computed its own progress at.
    const durations = modeDurationsAt(rentalId, current.json.server_time);
    assert.equal(durations.driving, 3 * PREPARED_DRIVING_SECONDS * 1_000_000);
    assert.equal(rental.progress.driving_duration_microseconds, String(durations.driving));
    assert.equal(rental.progress.paused_duration_microseconds, String(durations.paused));

    const drivingMinutes = expectedMinutes(durations.driving);
    const pausedMinutes = expectedMinutes(durations.paused);
    assert.equal(drivingMinutes, 2, 'the prepared driving intervals did not need a second minute');
    assert.equal(rental.progress.driving_started_minutes, String(drivingMinutes));
    assert.equal(rental.progress.paused_started_minutes, String(pausedMinutes));

    const snapshot = rental.tariff_snapshot;
    const amount =
      drivingMinutes * Number(snapshot.driving_rate_tyiyn_per_started_minute) +
      pausedMinutes * Number(snapshot.paused_rate_tyiyn_per_started_minute);
    assert.equal(rental.progress.estimated_amount_tyiyn, String(amount));
  });
});

describe('the amount a ride costs', () => {
  test('is the example of the specification, read from the intervals the database holds', async () => {
    const { account, rentalId } = await rideCommandRide('example');

    // Ninety seconds of driving and forty-five of pause: two begun minutes of driving and one of
    // pause, which at the demonstration rates of 1234 and 321 tyiyn is exactly 2789.
    prepareModeDurations(rentalId, EXAMPLE_DRIVING_SECONDS, EXAMPLE_PAUSED_SECONDS);

    const current = await currentOf(account);
    assert.equal(current.status, 200, current.text);
    const rental = current.json.rental;
    const durations = modeDurationsAt(rentalId, current.json.server_time);
    assert.equal(durations.driving, EXAMPLE_DRIVING_SECONDS * 1_000_000);
    assert.ok(
      durations.paused >= EXAMPLE_PAUSED_SECONDS * 1_000_000,
      `the pause the check prepared is ${durations.paused} microseconds`,
    );

    const snapshot = rental.tariff_snapshot;
    assert.equal(snapshot.driving_rate_tyiyn_per_started_minute, String(DRIVING_RATE_TYIYN));
    assert.equal(snapshot.paused_rate_tyiyn_per_started_minute, String(PAUSED_RATE_TYIYN));

    // The expected amount is computed from the two sums and the two stored rates, not written as a
    // constant: the check would still pass if the arithmetic of the service changed with them.
    const amount =
      expectedMinutes(durations.driving) * Number(snapshot.driving_rate_tyiyn_per_started_minute) +
      expectedMinutes(durations.paused) * Number(snapshot.paused_rate_tyiyn_per_started_minute);
    assert.equal(amount, EXAMPLE_AMOUNT_TYIYN);
    assert.equal(rental.progress.driving_started_minutes, '2');
    assert.equal(rental.progress.paused_started_minutes, '1');
    assert.equal(rental.progress.estimated_amount_tyiyn, String(EXAMPLE_AMOUNT_TYIYN));
  });

  test('is priced at the rates the reservation stored when the catalog moves afterwards', async () => {
    const { account, rentalId } = await rideCommandRide('snapshot');

    // Ninety seconds of driving, so that the amount has two begun minutes of driving to be wrong
    // about, and a pause of half a minute, which begins one minute with the whole of the check's own
    // delay to spare.
    prepareModeDurations(rentalId, EXAMPLE_DRIVING_SECONDS, SNAPSHOT_PAUSED_SECONDS);

    // The rates the rental stored are read, then replaced by a pair the demonstration never charges:
    // that is the catalog change of this check, standing in for an operator who changed the price
    // list after the reservation was made. The catalog itself still charges 1234 and 321 while the
    // check reads, so an amount computed from the catalog would not be the one the check accepts
    // below. Every value it asserts is derived from what it reads back, not written as a constant.
    const stored = { driving: DRIVING_RATE_TYIYN, paused: PAUSED_RATE_TYIYN };
    const moved = moveRentalRates(rentalId, MOVED_DRIVING_RATE_TYIYN, MOVED_PAUSED_RATE_TYIYN);
    assert.deepEqual(moved, stored, 'the rental stored rates other than the ones this check assumes');
    try {
      const rented = await currentOf(account);
      assert.equal(rented.status, 200, rented.text);

      const progress = rented.json.rental.progress;
      const snapshot = rented.json.rental.tariff_snapshot;
      assert.equal(
        snapshot.driving_rate_tyiyn_per_started_minute,
        String(MOVED_DRIVING_RATE_TYIYN),
        'the check did not move the rates the rental publishes',
      );
      assert.equal(Number(progress.driving_started_minutes), EXPECTED_DRIVING_STARTED_MINUTES);
      assert.ok(
        Number(progress.paused_started_minutes) >= 1,
        `the paused minutes are ${progress.paused_started_minutes}`,
      );
      assert.equal(
        progress.estimated_amount_tyiyn,
        amountOf(progress, snapshot),
        'the amount is not the minutes the answer publishes at the rates the snapshot stored',
      );
    } finally {
      restoreRentalRates(rentalId, moved);
    }

    // The rates are back where the check found them, so the suites after this one read the
    // demonstration rather than what a check left behind.
    assert.deepEqual(storedRentalRates(rentalId), stored);
  });

  test('keeps every digit of an amount no floating-point number can hold', async () => {
    const { account, rentalId } = await rideCommandRide('beyond-2-53');
    const moved = moveRentalRates(rentalId, BEYOND_THE_EXACT_DOUBLE_RANGE, 0);
    try {
      // One begun minute of driving at that rate is exactly that many tyiyn. A value that passed
      // through a number would arrive as 9007199254740992 or as a rounded neighbour.
      const answer = await currentOf(account);
      assert.equal(answer.status, 200, answer.text);

      // Read from the text of the answer rather than from the parsed body: what this check is about
      // is the digits the service published, and a parser is the thing that would lose them.
      const published = publishedInteger(answer.text, 'estimated_amount_tyiyn');
      assert.equal(published, String(BEYOND_THE_EXACT_DOUBLE_RANGE));
      assert.equal(BigInt(published), BigInt(BEYOND_THE_EXACT_DOUBLE_RANGE));
      // The digits that a double would have left behind, named here so the check shows what it is
      // about: this is the value the amount becomes on the way through a floating-point number.
      assert.equal(Number(published), 9_007_199_254_740_992);
      assert.equal(answer.json.rental.progress.estimated_amount_tyiyn, String(BEYOND_THE_EXACT_DOUBLE_RANGE));
      assert.equal(Number(answer.json.rental.progress.estimated_amount_tyiyn), Number(published));
    } finally {
      restoreRentalRates(rentalId, moved);
    }
  });
});

describe('a ride whose reservation deadline has passed', () => {
  test('is still current, still holds its vehicle and still takes a command', async () => {
    const { account, vehicleId, rentalId } = await reservedRide('past-deadline');
    const started = await rideCommand('start', rentalId, newCommandKey(), account);
    assert.equal(started.status, 200, started.text);

    moveDeadline(rentalId, -60);
    // Several passes of the deadline sweep fit in this wait, so a sweep that released the ride would
    // have done it before the assertions below read what it left.
    await delay(SWEEP_SETTLE_MS);

    const current = await currentOf(account);
    assert.equal(current.status, 200, current.text);
    assert.equal(current.json.kind, 'rental', 'a ride that has begun was answered as expired');
    assert.equal(current.json.rental.id, rentalId);
    assert.equal(current.json.rental.state, 'active');
    assert.equal(storedRental(rentalId)[0], 'active');

    const published = await publishedVehicle(vehicleId);
    assert.equal(published.status, 'in_trip', 'the vehicle of a ride past the deadline was released');
    assert.equal(published.ride_mode, 'driving');
    assert.equal(liveRentals('vehicle_id', vehicleId), 1);

    // The deadline of the reservation it came from decides nothing about a ride that has begun, so
    // the ride still answers the commands of its own lifecycle.
    const paused = await rideCommand('pause', rentalId, newCommandKey(), account);
    assert.equal(paused.status, 200, paused.text);
    assert.equal(paused.json.rental.state, 'paused');
  });
});

describe('a start on a vehicle that cannot begin', () => {
  test('refuses with the reasons and leaves the reservation fit to start once it can again', async () => {
    const { account, vehicleId, rentalId } = await reservedRide('unfit');
    const offered = await publishedVehicle(vehicleId);
    assert.ok(
      offered.energy_sources.some((source) => source.can_start),
      'the prepared vehicle could not start before the check changed it',
    );

    const replaced = drainVehicle(vehicleId);
    try {
      // The reservation still holds the vehicle, so the catalog publishes it as reserved while every
      // one of its sources has stopped permitting a start.
      const spent = await publishedVehicle(vehicleId);
      assert.equal(spent.status, 'reserved', 'a refusal of this check released the vehicle');
      assert.ok(
        spent.energy_sources.every((source) => !source.can_start),
        'a source still permits a start',
      );

      const answer = await rideCommand('start', rentalId, newCommandKey(), account);
      assert.equal(answer.status, 409, answer.text);
      assert.equal(answer.json.code, 'VEHICLE_UNAVAILABLE', answer.text);
      assert.ok(answer.json.details, `the refusal carried no details: ${answer.text}`);
      assert.ok(
        answer.json.details.unavailable_reasons.includes('insufficient_energy'),
        `the refusal named ${JSON.stringify(answer.json.details.unavailable_reasons)}`,
      );

      assert.equal(storedRental(rentalId)[0], 'reserved', 'the refusal ended the reservation');
      assert.deepEqual(intervalsOf(rentalId), [], 'the refusal opened an interval');
    } finally {
      restoreRemainders(vehicleId, replaced);
    }

    // The refusal neither cancelled nor shortened the reservation, so the same reservation starts as
    // soon as a source can move the vehicle again.
    const started = await rideCommand('start', rentalId, newCommandKey(), account);
    assert.equal(started.status, 200, started.text);
    assert.equal(started.json.rental.state, 'active');
  });
});

/** Registers an account and takes one free vehicle with it, which every check above starts from. */
async function reservedRide(name) {
  const account = await newAccount(`${ACCOUNT_PREFIX}-${name}`);
  const vehicleId = await availableVehicle();
  const created = await reserve(vehicleId, newCommandKey(), account);
  assert.equal(created.status, 201, created.text);
  return { account, vehicleId, rentalId: created.json.rental.id };
}

/**
 * Takes a vehicle and drives it into the paused stage, which is the state every check of an amount
 * starts from: a paused ride holds one open interval, so the durations of both modes can be prepared
 * without the open one growing while the check reads.
 */
async function rideCommandRide(name) {
  const reserved = await reservedRide(name);
  for (const operation of ['start', 'pause']) {
    const answer = await rideCommand(operation, reserved.rentalId, newCommandKey(), reserved.account);
    assert.equal(answer.status, 200, answer.text);
  }
  return reserved;
}

/** Sends one ride command for one rental and reports the answer, whatever it is. */
function rideCommand(operation, rentalId, key, account) {
  return call(RIDE_PATHS[operation](rentalId), {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: key },
  });
}

/**
 * Every interval the database holds for one rental, oldest first, with both moments in whole
 * microseconds. The moments are compared as numbers rather than as the text the contract publishes,
 * because the invariant is stated to the microsecond and text would leave it to a string comparison.
 */
function intervalsOf(rentalId) {
  const rows = sql(
    `SELECT mode || '|' || (extract(epoch FROM started_at) * 1000000)::bigint || '|' || ` +
      `coalesce((extract(epoch FROM ended_at) * 1000000)::bigint::text, '') ` +
      `FROM ride_segments WHERE rental_id = '${rentalId}' ORDER BY started_at, id`,
  );
  if (rows === '') return [];
  return rows.split('\n').map((row) => {
    const [mode, startedAt, endedAt] = row.split('|');
    return { mode, startedAt: Number(startedAt), endedAt: endedAt === '' ? null : Number(endedAt) };
  });
}

/**
 * The intervals of one rental and the invariant every transition must leave behind: exactly one of
 * them open, each beginning at the microsecond the one before it ended, and none of no length.
 */
function assertOneOpenInterval(rentalId, complaint) {
  const intervals = intervalsOf(rentalId);
  const open = intervals.filter((interval) => interval.endedAt === null);
  assert.equal(open.length, 1, `${complaint} the rental holds ${open.length} open intervals`);

  for (const [index, interval] of intervals.entries()) {
    if (interval.endedAt !== null) {
      assert.ok(interval.endedAt > interval.startedAt, `${complaint} interval ${index} lasts no time`);
    }
  }
  for (let index = 1; index < intervals.length; index += 1) {
    assert.equal(
      intervals[index].startedAt,
      intervals[index - 1].endedAt,
      `${complaint} interval ${index} begins at ${intervals[index].startedAt} instead of ${intervals[index - 1].endedAt}`,
    );
  }
  return intervals;
}

/** One stored moment of a rental, rendered the way the contract publishes it. */
function storedMoment(rentalId, column) {
  return sql(
    `SELECT to_char(${column} AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM rentals WHERE id = '${rentalId}'`,
  );
}

/** The moment one rental publishes as the start of its current mode, in whole microseconds. */
function modeStartMicroseconds(rentalId) {
  return Number(
    sql(`SELECT (extract(epoch FROM mode_started_at) * 1000000)::bigint FROM rentals WHERE id = '${rentalId}'`),
  );
}

/** Asserts that a duration a command answered with is one that has only just begun. */
function assertSmallDuration(published, complaint) {
  const microseconds = Number(published);
  assert.ok(Number.isInteger(microseconds), `${complaint} published ${published}`);
  assert.ok(
    microseconds >= 0 && microseconds < JUST_BEGUN_MICROSECONDS,
    `${complaint} published ${published} microseconds`,
  );
}

/** The beginning of a summed duration, which the policy rounds up once for the whole mode. */
function expectedMinutes(microseconds) {
  return Math.ceil(microseconds / MICROSECONDS_PER_MINUTE);
}

/**
 * The duration of each mode one ride's intervals hold at one moment, in whole microseconds, summed
 * per mode the way the billing policy sums them. The moment is the one an answer stated, so the
 * durations are compared with the sums the service computed at exactly that instant.
 */
function modeDurationsAt(rentalId, moment) {
  const rows = sql(
    `SELECT mode || '|' || ` +
      `coalesce((extract(epoch FROM sum(coalesce(ended_at, TIMESTAMPTZ '${moment}') - started_at)) * 1000000)::bigint, 0) ` +
      `FROM ride_segments WHERE rental_id = '${rentalId}' AND started_at <= TIMESTAMPTZ '${moment}' GROUP BY mode`,
  );
  const durations = { driving: 0, paused: 0 };
  for (const row of rows === '' ? [] : rows.split('\n')) {
    const [mode, microseconds] = row.split('|');
    durations[mode] = Number(microseconds);
  }
  return durations;
}

/**
 * Gives every interval of one ride the duration the check of the published progress compares against,
 * keeping the chain continuous and the moment the rental publishes as its current mode naming the
 * open interval. The moments are written rather than waited for: a ride with begun minutes in both
 * modes would otherwise cost minutes of wall-clock time.
 */
function prepareIntervals(rentalId) {
  sql(
    `WITH ride AS (
       SELECT segment.id,
              segment.ended_at IS NULL AS open,
              row_number() OVER (ORDER BY segment.started_at, segment.id) AS position,
              CASE segment.mode
                WHEN 'driving' THEN ${PREPARED_DRIVING_SECONDS}::double precision
                ELSE ${PREPARED_PAUSED_SECONDS}::double precision
              END AS seconds
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
 * Gives every interval of one ride the duration the check of an amount compares against, keeping the
 * chain continuous. The moments are written rather than waited for, because a ride of two begun
 * minutes would otherwise cost two minutes of wall-clock time.
 *
 * The chain ends at the present moment, so the ride is entirely in the past and the sums are the ones
 * stated here however long the check takes. Anchoring it to the moment the ride began would place
 * every interval before the ride did, and the service, reading at a later moment, would count only
 * the overlap — a ride driving for ninety seconds and paused for eighty shares none of its pause with
 * the moment the ride began.
 *
 * Only two intervals are expected: a ride that has been started and paused holds exactly one of each
 * mode, so the durations the check states are the sums the service reads back.
 */
function prepareModeDurations(rentalId, drivingSeconds, pausedSeconds) {
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
 * Moves the rates one rental was reserved under, which is how this suite stands in for an operator
 * changing the catalog after a reservation was made, and answers what the rental stored before. The
 * rates are written on the rental rather than read from the price list, because those are the ones a
 * ride is priced at.
 */
function moveRentalRates(rentalId, drivingRateTyiyn, pausedRateTyiyn) {
  const replaced = storedRentalRates(rentalId);
  sql(
    `UPDATE rentals SET ` +
      `tariff_driving_rate_tyiyn_per_started_minute = ${drivingRateTyiyn}, ` +
      `tariff_paused_rate_tyiyn_per_started_minute = ${pausedRateTyiyn} ` +
      `WHERE id = '${rentalId}'`,
  );
  return replaced;
}

/** The rates one rental was reserved under, as the columns hold them. */
function storedRentalRates(rentalId) {
  const rates = sql(
    `SELECT tariff_driving_rate_tyiyn_per_started_minute, ` +
      `tariff_paused_rate_tyiyn_per_started_minute ` +
      `FROM rentals WHERE id = '${rentalId}'`,
  );
  assert.notEqual(rates, '', 'the rental stores no rates to read');
  const [driving, paused] = rates.split('|');
  return { driving: Number(driving), paused: Number(paused) };
}

/** Puts back the rates moveRentalRates replaced. */
function restoreRentalRates(rentalId, replaced) {
  sql(
    `UPDATE rentals SET ` +
      `tariff_driving_rate_tyiyn_per_started_minute = ${replaced.driving}, ` +
      `tariff_paused_rate_tyiyn_per_started_minute = ${replaced.paused} ` +
      `WHERE id = '${rentalId}'`,
  );
}

/**
 * What a progress costs at the rates of a snapshot: the minutes the answer published of each mode
 * times the rate of that mode. The multiplication is a check's own arithmetic, so it is done as a
 * number; nothing a service publishes is read through one.
 */
function amountOf(progress, snapshot) {
  return String(
    Number(progress.driving_started_minutes) * Number(snapshot.driving_rate_tyiyn_per_started_minute) +
      Number(progress.paused_started_minutes) * Number(snapshot.paused_rate_tyiyn_per_started_minute),
  );
}

/**
 * One whole number as the answer published it, read from the text of the body rather than from the
 * parsed one. A value the contract carries as a decimal string is exactly what a check about losing
 * digits has to read, and a parser is the thing that would lose them.
 */
function publishedInteger(body, field) {
  const published = new RegExp(`"${field}":"([0-9]+)"`).exec(body);
  assert.notEqual(published, null, `the answer publishes no ${field}: ${body}`);
  return published[1];
}

/**
 * Leaves every source of one vehicle below the start threshold and reports what it replaced, so the
 * check can put the reserves back exactly as they were. Two sources below the threshold are not one
 * source above it, which is the rule the vehicle's suitability is decided by.
 */
function drainVehicle(vehicleId) {
  const replaced = sql(
    `SELECT source_kind || '|' || remaining FROM vehicle_energy_sources ` +
      `WHERE vehicle_id = '${vehicleId}' ORDER BY source_kind`,
  );
  assert.notEqual(replaced, '', 'the chosen vehicle holds no energy source');
  sql(
    `UPDATE vehicle_energy_sources SET remaining = capacity * ${DRAINED_BASIS_POINTS} / 10000 ` +
      `WHERE vehicle_id = '${vehicleId}'`,
  );
  return replaced.split('\n');
}

/** Puts back the reserves drainVehicle replaced. */
function restoreRemainders(vehicleId, replaced) {
  for (const row of replaced) {
    const [kind, remaining] = row.split('|');
    sql(
      `UPDATE vehicle_energy_sources SET remaining = ${remaining} ` +
        `WHERE vehicle_id = '${vehicleId}' AND source_kind = '${kind}'`,
    );
  }
}

/**
 * Ends everything this suite wrote: the signals about the notifications of its rentals, then the
 * reservations themselves through the service, and finally the rows. An interval goes with its
 * rental, because an interval is a fact about one rental and has no meaning without it.
 */
async function endSuiteRides() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(`DELETE FROM outbox WHERE resource_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(
    `DELETE FROM outbox WHERE resource_id IN (
       SELECT note.id FROM notifications note
       JOIN rentals rental ON rental.id = note.rental_id
       WHERE rental.user_id IN ${mine})`,
  );
  await endSuiteReservations();
}
