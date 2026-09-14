// Finishing a ride on the assembled stack: the command over the real HTTP boundary, the invoice it
// issues against the intervals the database holds, the area check against real PostGIS, and the
// refusals an ending meets where it stands.
//
// The positions are moved with direct SQL rather than waited for, as the ride suites prepare their
// intervals: a vehicle that drives out of the area would otherwise cost minutes of wall-clock time,
// and the moment of a confirmed reading cannot be reached by waiting at all. The demonstration
// confirms the whole fleet every five seconds, so a check that moves a position sends its command in
// the same breath and puts the position back afterwards.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import { waitForReady } from './client.mjs';
import {
  IDEMPOTENCY_HEADER,
  availableVehicle,
  call,
  currentOf,
  endSuiteReservations,
  liveRentals,
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
const ACCOUNT_PREFIX = 'finishing';

/** Where the command is sent, which is the path its idempotency fingerprint covers. */
const finishPath = (rentalId) => `/api/v1/rides/${rentalId}/finish`;

/** The moment the contract publishes, which every stored moment is compared against. */
const MOMENT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;

const MICROSECONDS_PER_MINUTE = 60_000_000;

/** The rates the demonstration charges, which every reservation stores in its snapshot. */
const DRIVING_RATE_TYIYN = 1234;
const PAUSED_RATE_TYIYN = 321;

/**
 * Two points on and beside the edge of the seeded service area, which is the rectangle
 * [74.55, 42.84]-[74.65, 42.90]. The rule is stated as coordinates rather than as a distance, so a
 * reader sees the boundary the database decides: a position on an edge is covered and one ten
 * millionths of a degree west of it is not.
 */
const ON_THE_EDGE = { longitude: 74.55, latitude: 42.87 };
const OUTSIDE_THE_AREA = { longitude: 74.549999, latitude: 42.87 };

/** An amount past the exact range of a double, which a client must receive digit for digit. */
const BEYOND_THE_EXACT_DOUBLE_RANGE = 9_007_199_254_740_993;

/**
 * The durations the checks of an invoice give the two intervals, in seconds. A paused ride holds one
 * open interval, so the durations are prepared rather than waited for: ninety seconds of driving and
 * forty-five of pause are two begun minutes and one, which is the example the product states.
 */
const PREPARED_DRIVING_SECONDS = 90;
const PREPARED_PAUSED_SECONDS = 45;

before(async () => {
  await waitForReady();
});

// Every check below takes a vehicle, so what one left holding is given back before the next starts.
beforeEach(async () => {
  await endSuiteFinishes();
});

// The checks move positions and rates the prepared scenario owns, so the suite puts the
// demonstration back rather than leaving the next reader a fleet it changed.
after(async () => {
  await endSuiteFinishes();
  restoreScenario();
});

describe('finishing a ride', () => {
  test('answers 200 with the completed ride, one invoice and a closed interval', async () => {
    const { account, vehicleId, rentalId } = await riding('inside');

    const answer = await finishCommand(rentalId, newCommandKey(), account);
    assert.equal(answer.status, 200, answer.text);
    assert.match(answer.json.server_time, MOMENT);

    const rental = answer.json.rental;
    assert.equal(rental.id, rentalId);
    assert.equal(rental.state, 'completed');
    assert.equal(rental.completion.reason, 'user_finished');
    assert.equal(rental.completed_at, answer.json.server_time, 'the ride ended at another moment');
    assert.equal(rental.invoice_id, answer.json.invoice.invoice.id);
    assert.equal(rental.vehicle.status, 'available', 'the vehicle of the finished ride is still held');
    assert.equal(rental.vehicle.ride_mode, undefined, 'a free vehicle still publishes a ride mode');

    // The ending is what the database holds rather than only what the answer says: the stage, the
    // reason, the moment and the interval the ride was in when it ended.
    const stored = storedRental(rentalId)[0];
    assert.equal(stored, 'completed');
    assert.equal(storedMoment(rentalId, 'ended_at'), rental.completed_at);
    assert.equal(completionReasonOf(rentalId), 'user_finished');
    assert.equal(openIntervals(rentalId), 0, 'the finished ride still holds an open interval');
    assert.equal(intervalsOf(rentalId).at(-1).endedAt, storedMicroseconds(rentalId, 'ended_at'));

    assert.equal(liveRentals('vehicle_id', vehicleId), 0, 'the finished ride still holds its vehicle');
    assert.equal((await publishedVehicle(vehicleId)).status, 'available');
  });

  test('publishes an invoice equal to the intervals the database holds', async () => {
    const { account, rentalId } = await riding('invoice');

    // The intervals are given a length of their own, as the ride suites prepare them: an invoice of a
    // ride that lasted a second has nothing to be wrong about, and a minute of wall-clock time per
    // check is not a price worth paying for a longer one.
    prepareIntervals(rentalId, PREPARED_DRIVING_SECONDS, PREPARED_PAUSED_SECONDS);

    const answer = await finishCommand(rentalId, newCommandKey(), account);
    assert.equal(answer.status, 200, answer.text);

    // The durations the invoice states are the sums the intervals hold by the moment the ride ended,
    // and the amount of each line is those minutes at the rate the rental stored. Every number the
    // check asserts is derived from what it read, so a change of the rates or of the intervals moves
    // both sides together instead of breaking the check.
    const invoice = answer.json.invoice.invoice;
    const endedAt = answer.json.rental.completed_at;
    const durations = modeDurationsAt(rentalId, endedAt);
    const rates = storedRates(rentalId);

    assert.equal(invoice.rental_id, rentalId);
    assert.equal(invoice.currency, 'KGS');
    assert.equal(invoice.billing_policy, 'per_mode_started_minute_v1');
    assert.equal(invoice.completion.reason, 'user_finished');
    assert.equal(invoice.issued_at, endedAt, 'the invoice was issued at another moment');
    assert.equal(invoice.lines.length, 2, 'the invoice does not state exactly two lines');
    assert.deepEqual(
      invoice.lines.map((line) => line.mode),
      ['driving', 'paused'],
      'the invoice publishes its lines in another order',
    );

    for (const [mode, line] of [
      ['driving', invoice.lines[0]],
      ['paused', invoice.lines[1]],
    ]) {
      // The minutes are derived from the duration the invoice itself states, rounded up once for the
      // whole mode: an amount that did not follow from the duration beside it is the defect a reader
      // adding up the invoice would find.
      const expected = expectedLine(Number(line.duration_microseconds), rates[mode]);
      assert.equal(line.duration_microseconds, String(durations[mode]), `${mode} duration`);
      assert.equal(line.billed_started_minutes, String(expected.minutes), `${mode} minutes`);
      assert.equal(line.rate_tyiyn_per_started_minute, String(rates[mode]), `${mode} rate`);
      assert.equal(line.amount_tyiyn, String(expected.amount), `${mode} amount`);
    }

    const total = invoice.lines.reduce((sum, line) => sum + Number(line.amount_tyiyn), 0);
    assert.equal(invoice.total_amount_tyiyn, String(total), 'the invoice does not add up');
    assert.equal(answer.json.invoice.payment.status, 'pending');
    assert.equal(answer.json.invoice.version, '1');
  });

  test('keeps every digit of an amount no floating-point number can hold', async () => {
    const { account, rentalId } = await riding('beyond-2-53');
    prepareIntervals(rentalId, PREPARED_DRIVING_SECONDS, PREPARED_PAUSED_SECONDS);
    const moved = moveRentalRates(rentalId, BEYOND_THE_EXACT_DOUBLE_RANGE, 1);
    try {
      // Two begun driving minutes at that rate are 18014398509481986 tyiyn, which no double holds: read
      // as a number the answer would arrive as a rounded neighbour. The digits are therefore read from
      // the text of the answer, and the product is proven with BigInt rather than against a constant
      // this check would have to be trusted about.
      const answer = await finishCommand(rentalId, newCommandKey(), account);
      assert.equal(answer.status, 200, answer.text);

      const driving = answer.json.invoice.invoice.lines[0];
      const minutes = BigInt(driving.billed_started_minutes);
      const rate = BigInt(driving.rate_tyiyn_per_started_minute);
      assert.equal(rate, BigInt(BEYOND_THE_EXACT_DOUBLE_RANGE), 'the check did not move the rate');
      assert.ok(minutes >= 2n, `the check gave the driving mode ${minutes} begun minutes`);

      const expected = minutes * rate + BigInt(answer.json.invoice.invoice.lines[1].amount_tyiyn);
      const published = publishedInteger(answer.text, 'total_amount_tyiyn');
      assert.equal(published, expected.toString(), 'the published digits are not the product');
      assert.ok(
        BigInt(published) > BigInt(Number.MAX_SAFE_INTEGER),
        `the amount ${published} is one a double could hold, so this check proves nothing`,
      );
      // The digits that a double would have left behind, named here so the check shows what it is
      // about: this is the value the amount becomes on the way through a floating-point number.
      assert.notEqual(String(Number(published)), published);
    } finally {
      restoreRentalRates(rentalId, moved);
    }
    assert.deepEqual(storedRates(rentalId), moved, 'the check left the rates of the ride moved');
  });
});

describe('repeating a finish', () => {
  test('answers the stored body with the replay header and issues no second invoice', async () => {
    const { account, rentalId } = await riding('repeat');
    const key = newCommandKey();

    const first = await finishCommand(rentalId, key, account);
    assert.equal(first.status, 200, first.text);
    const invoices = invoiceCount(rentalId);

    const repeat = await finishCommand(rentalId, key, account);
    assert.equal(repeat.status, 200, repeat.text);
    assert.equal(repeat.headers.get('Idempotency-Replayed'), 'true', 'the repeat was not marked as one');
    assert.deepEqual(repeat.json, first.json, 'the repeat recomputed its answer');
    assert.equal(invoiceCount(rentalId), invoices);
  });

  test('answers a new key with the ending that was stored and issues no second invoice', async () => {
    const { account, vehicleId, rentalId } = await riding('new-key');

    const first = await finishCommand(rentalId, newCommandKey(), account);
    assert.equal(first.status, 200, first.text);
    const invoices = invoiceCount(rentalId);
    assert.equal(invoices, 1);

    // The contract states that a new key for a completed ride returns the existing rental and
    // invoice, so this is not a refusal and not a second ending: the reason, the moment and the
    // invoice are the ones the first attempt stored.
    const again = await finishCommand(rentalId, newCommandKey(), account);
    assert.equal(again.status, 200, again.text);
    assert.equal(again.json.rental.completed_at, first.json.rental.completed_at);
    assert.equal(again.json.rental.completion.reason, first.json.rental.completion.reason);
    assert.equal(again.json.invoice.invoice.id, first.json.invoice.invoice.id);
    assert.equal(again.json.rental.version, first.json.rental.version, 'the repeat moved the version');
    assert.equal(invoiceCount(rentalId), invoices, 'the repeat issued a second invoice');
    assert.equal(liveRentals('vehicle_id', vehicleId), 0);
  });

  test('leaves one completed ride and one invoice when two finishes race', async () => {
    const { account, rentalId } = await riding('race');

    const answers = await race([
      () => finishCommand(rentalId, newCommandKey(), account),
      () => finishCommand(rentalId, newCommandKey(), account),
    ]);

    for (const answer of answers) {
      assert.equal(answer.status, 200, answer.text);
      assert.equal(answer.json.rental.state, 'completed');
    }
    assert.equal(storedRental(rentalId)[0], 'completed');
    assert.equal(invoiceCount(rentalId), 1, 'a race issued more than one invoice');
    assert.equal(openIntervals(rentalId), 0);
    // Both answers describe the one ending rather than two: they cannot state two different moments.
    assert.equal(answers[0].json.rental.completed_at, answers[1].json.rental.completed_at);
    assert.equal(answers[0].json.invoice.invoice.id, answers[1].json.invoice.invoice.id);
  });
});

describe('an ending outside the service area', () => {
  test('is refused, keeps the ride charging and keeps its interval open', async () => {
    const { account, vehicleId, rentalId } = await riding('outside');
    const replaced = movePosition(vehicleId, OUTSIDE_THE_AREA);
    try {
      assert.equal((await publishedVehicle(vehicleId)).status, 'in_trip');

      const answer = await finishCommand(rentalId, newCommandKey(), account);
      assert.equal(answer.status, 409, answer.text);
      assert.equal(answer.json.code, 'OUTSIDE_SERVICE_ZONE', answer.text);

      // A refused ending changes nothing: the ride keeps its mode, its open interval and its
      // charging, which is what "the ride continues to be billed" means in rows.
      const stored = storedRental(rentalId)[0];
      assert.equal(stored, 'paused', `the refused ending left the ride ${stored}`);
      assert.equal(openIntervals(rentalId), 1, 'the refused ending closed the interval of the ride');
      assert.equal(completionReasonOf(rentalId), '', 'the refused ending recorded a reason');
      assert.equal(invoiceCount(rentalId), 0, 'the refused ending issued an invoice');
      assert.equal(liveRentals('vehicle_id', vehicleId), 1);
      assert.equal((await publishedVehicle(vehicleId)).ride_mode, 'paused');
    } finally {
      restorePosition(vehicleId, replaced);
    }

    // With the position back inside the area the same ride ends, so the refusal was about where the
    // vehicle stood rather than about the ride.
    const ended = await finishCommand(rentalId, newCommandKey(), account);
    assert.equal(ended.status, 200, ended.text);
    assert.equal(ended.json.rental.state, 'completed');
  });

  test('is allowed on the edge of the area, which the coverage test counts as inside', async () => {
    const { account, vehicleId, rentalId } = await riding('on-edge');
    const replaced = movePosition(vehicleId, ON_THE_EDGE);
    try {
      const answer = await finishCommand(rentalId, newCommandKey(), account);
      assert.equal(answer.status, 200, answer.text);
      assert.equal(answer.json.rental.state, 'completed');
    } finally {
      restorePosition(vehicleId, replaced);
    }
  });
});

describe('an ending on a position that cannot be trusted', () => {
  test('is refused and keeps the ride charging', async () => {
    const { account, vehicleId, rentalId } = await riding('stale');
    const replaced = agePosition(vehicleId);
    try {
      // The demonstration confirms the whole fleet every five seconds, so a reading this check ages is
      // repaired by the next pass of that source. The command is sent immediately after the reading is
      // aged, and the check tries again rather than reporting a race as a missing rule.
      const answer = await staleFinish(account, rentalId, vehicleId);
      assert.equal(answer.status, 409, answer.text);
      assert.equal(answer.json.code, 'TELEMETRY_STALE', answer.text);

      assert.equal(storedRental(rentalId)[0], 'paused');
      assert.equal(openIntervals(rentalId), 1);
      assert.equal(invoiceCount(rentalId), 0);
    } finally {
      restoreReading(vehicleId, replaced);
    }
  });
});

/**
 * Sends a finish and answers it, re-ageing the reading when the demonstration repaired it before the
 * command arrived. A refusal that names the position is what this check is about; an answer that says
 * the position was fresh is the demonstration having won a race, which is retried rather than reported
 * as a rule that does not hold.
 */
async function staleFinish(account, rentalId, vehicleId) {
  for (let attempt = 1; ; attempt += 1) {
    agePosition(vehicleId);
    const answer = await finishCommand(rentalId, newCommandKey(), account);
    const refreshed = answer.json?.code === 'OUTSIDE_SERVICE_ZONE';
    if (!refreshed || attempt === STALE_ATTEMPTS) return answer;
  }
}

/** How many times a check re-ages a reading that the demonstration repaired before the command. */
const STALE_ATTEMPTS = 3;

describe('access to finishing a ride', () => {
  test('refuses a request without a session and a ride that belongs to somebody else', async () => {
    const { account, rentalId } = await riding('access');

    const anonymous = await call(finishPath(rentalId), {
      method: 'POST',
      csrfToken: 'a-token-no-session-holds',
      headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
    });
    assert.equal(anonymous.status, 401, anonymous.text);

    const stranger = await newAccount(`${ACCOUNT_PREFIX}-stranger`);
    const foreign = await finishCommand(rentalId, newCommandKey(), stranger);
    assert.equal(foreign.status, 404, foreign.text);
    assert.equal(foreign.json.code, 'RESOURCE_NOT_FOUND', foreign.text);
    assert.equal(storedRental(rentalId)[0], 'paused', "a stranger ended somebody else's ride");
    assert.equal(invoiceCount(rentalId), 0);

    // The ride of the account that owns it is untouched by the two refusals above.
    const own = await finishCommand(rentalId, newCommandKey(), account);
    assert.equal(own.status, 200, own.text);
  });

  test('refuses the finish of a reservation with the state code', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-reservation`);
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);

    const answer = await finishCommand(created.json.rental.id, newCommandKey(), account);
    assert.equal(answer.status, 409, answer.text);
    assert.equal(answer.json.code, 'INVALID_RENTAL_STATE', answer.text);
    assert.equal(storedRental(created.json.rental.id)[0], 'reserved');
    assert.equal(invoiceCount(created.json.rental.id), 0);
  });
});

describe('a ride that has ended', () => {
  test('leaves the vehicle free for another account to take', async () => {
    const { account, vehicleId, rentalId } = await riding('release');
    const ended = await finishCommand(rentalId, newCommandKey(), account);
    assert.equal(ended.status, 200, ended.text);

    const other = await newAccount(`${ACCOUNT_PREFIX}-next`);
    const taken = await reserve(vehicleId, newCommandKey(), other);
    assert.equal(taken.status, 201, taken.text);
    assert.equal(taken.json.rental.vehicle.id, vehicleId);
    assert.equal(liveRentals('vehicle_id', vehicleId), 1);
  });

  test('is not current any more, and its invoice is still readable as it was issued', async () => {
    const { account, rentalId } = await riding('not-current');
    const ended = await finishCommand(rentalId, newCommandKey(), account);
    assert.equal(ended.status, 200, ended.text);

    const current = await currentOf(account);
    assert.equal(current.status, 200, current.text);
    assert.equal(current.json.kind, 'none', 'a finished ride is still published as current');

    // The invoice is immutable: what the answer published is what the row holds, and reading it
    // again through the same ending answers the same digits.
    const again = await finishCommand(rentalId, newCommandKey(), account);
    assert.deepEqual(again.json.invoice.invoice, ended.json.invoice.invoice);
  });
});

/** Registers an account, takes one free vehicle and drives it into the paused stage. */
async function riding(name) {
  const account = await newAccount(`${ACCOUNT_PREFIX}-${name}`);
  const vehicleId = await availableVehicle();
  const created = await reserve(vehicleId, newCommandKey(), account);
  assert.equal(created.status, 201, created.text);
  const rentalId = created.json.rental.id;

  const started = await rideCommand('start', rentalId, account);
  assert.equal(started.status, 200, started.text);
  const paused = await rideCommand('pause', rentalId, account);
  assert.equal(paused.status, 200, paused.text);

  return { account, vehicleId, rentalId };
}

/** Sends one finish for one rental and reports the answer, whatever it is. */
function finishCommand(rentalId, key, account) {
  return call(finishPath(rentalId), {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: key },
  });
}

/** Sends one ride command for one rental, which the checks above use to reach a ride in progress. */
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

/**
 * The minutes a duration has begun under the policy, which the check computes for itself rather than
 * reading from the service: the invoice is compared with an independently rounded sum, not with the
 * number the service published beside it.
 */
function expectedLine(microseconds, rateTyiyn) {
  const minutes = Math.ceil(microseconds / MICROSECONDS_PER_MINUTE);
  return { minutes, amount: minutes * rateTyiyn };
}

/**
 * Gives each interval of one paused ride the duration a check states, keeping the chain continuous and
 * the moment the rental publishes as its current mode naming the open interval. The moments are written
 * rather than waited for: a ride of two begun minutes would otherwise cost two minutes of wall-clock
 * time, and the ride is finished immediately afterwards, so the open interval is closed at the moment
 * the chain ends rather than growing while the check reads.
 *
 * Only two intervals are expected: a ride that has been started and paused holds exactly one of each
 * mode, so the durations stated here are the sums the service prices.
 */
function prepareIntervals(rentalId, drivingSeconds, pausedSeconds) {
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

/** The rates one rental was reserved under, which is what an invoice states. */
function storedRates(rentalId) {
  const rates = storedRental(rentalId);
  return { driving: Number(rates[7]), paused: Number(rates[8]) };
}

/** The interval of one ride that is still open, of which a finished ride has none. */
function openIntervals(rentalId) {
  return Number(sql(`SELECT count(*) FROM ride_segments WHERE rental_id = '${rentalId}' AND ended_at IS NULL`));
}

/** How many invoices one ride holds, which a repeated ending must not add to. */
function invoiceCount(rentalId) {
  return Number(sql(`SELECT count(*) FROM invoices WHERE rental_id = '${rentalId}'`));
}

/** Why one ride ended, as the row states it, or the empty string while it has not. */
function completionReasonOf(rentalId) {
  return sql(`SELECT coalesce(completion_reason, '') FROM rentals WHERE id = '${rentalId}'`);
}

/** One stored moment of a rental, rendered the way the contract publishes it. */
function storedMoment(rentalId, column) {
  return sql(
    `SELECT to_char(${column} AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') ` +
      `FROM rentals WHERE id = '${rentalId}'`,
  );
}

/** One stored moment in the whole microseconds the intervals of a ride are compared in. */
function storedMicroseconds(rentalId, column) {
  return Number(sql(`SELECT (extract(epoch FROM ${column}) * 1000000)::bigint FROM rentals WHERE id = '${rentalId}'`));
}

/**
 * Every interval the database holds for one rental, oldest first, with both moments in whole
 * microseconds, which is the unit the invariant of continuity is stated in. The moments are read as
 * counts rather than as the text the contract publishes, because a comparison to the microsecond is
 * what the invariant is about and a formatted string would round it.
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
 * The duration of each mode one ride's intervals hold by one moment, in whole microseconds, summed per
 * mode the way the billing policy sums them. The moment is derived from the moment the answer states
 * rather than from the clock, so the durations are compared with the sums the service computed at
 * exactly that instant.
 */
function modeDurationsAt(rentalId, moment) {
  const at = `TIMESTAMPTZ '${moment}'`;
  const rows = sql(
    `SELECT mode || '|' || ` +
      `coalesce((extract(epoch FROM sum(coalesce(ended_at, ${at}) - started_at)) * 1000000)::bigint, 0) ` +
      `FROM ride_segments WHERE rental_id = '${rentalId}' AND started_at <= ${at} GROUP BY mode`,
  );
  const durations = { driving: 0, paused: 0 };
  for (const row of rows === '' ? [] : rows.split('\n')) {
    const [mode, microseconds] = row.split('|');
    durations[mode] = Number(microseconds);
  }
  return durations;
}

/**
 * Moves the confirmed position of one vehicle, which is how this suite stands in for a vehicle that
 * drove out of the service area or onto its edge, and answers what the position was.
 */
function movePosition(vehicleId, position) {
  const replaced = sql(
    `SELECT ST_X(position) || '|' || ST_Y(position) FROM vehicle_telemetry ` + `WHERE vehicle_id = '${vehicleId}'`,
  );
  assert.notEqual(replaced, '', 'the chosen vehicle has confirmed no position');
  sql(
    `UPDATE vehicle_telemetry SET position = ST_SetSRID(ST_MakePoint(${position.longitude}, ${position.latitude}), 4326) ` +
      `WHERE vehicle_id = '${vehicleId}'`,
  );
  return replaced;
}

/** Puts back the position movePosition replaced. */
function restorePosition(vehicleId, replaced) {
  const [longitude, latitude] = replaced.split('|');
  sql(
    `UPDATE vehicle_telemetry SET position = ST_SetSRID(ST_MakePoint(${longitude}, ${latitude}), 4326) ` +
      `WHERE vehicle_id = '${vehicleId}'`,
  );
}

/**
 * Ages the confirmed reading of one vehicle past the limit the freshness rule states, and answers what
 * the row held. The demonstration confirms the fleet every five seconds, so the check reads the
 * refusal before its next pass; the reading is put back afterwards either way.
 */
function agePosition(vehicleId) {
  const replaced = sql(
    `SELECT to_char(confirmed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') ` +
      `FROM vehicle_telemetry WHERE vehicle_id = '${vehicleId}'`,
  );
  assert.notEqual(replaced, '', 'the chosen vehicle has confirmed no position');
  sql(
    `UPDATE vehicle_telemetry SET confirmed_at = confirmed_at - interval '16 seconds' ` +
      `WHERE vehicle_id = '${vehicleId}'`,
  );
  return replaced;
}

/** Puts back the reading agePosition moved, which is stated as the moment it held. */
function restoreReading(vehicleId, replaced) {
  sql(`UPDATE vehicle_telemetry SET confirmed_at = TIMESTAMPTZ '${replaced}' ` + `WHERE vehicle_id = '${vehicleId}'`);
}

/**
 * Moves the rates one rental was reserved under, which is how this suite proves an invoice states the
 * price list of its own ride, and answers what the rental stored before.
 */
function moveRentalRates(rentalId, drivingRateTyiyn, pausedRateTyiyn) {
  const replaced = storedRates(rentalId);
  sql(
    `UPDATE rentals SET ` +
      `tariff_driving_rate_tyiyn_per_started_minute = ${drivingRateTyiyn}, ` +
      `tariff_paused_rate_tyiyn_per_started_minute = ${pausedRateTyiyn} ` +
      `WHERE id = '${rentalId}'`,
  );
  return replaced;
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
 * One whole number as the answer published it, read from the text of the body rather than from the
 * parsed one: a value the contract carries as a decimal string is exactly what a check about losing
 * digits has to read, and a parser is the thing that would lose them.
 */
function publishedInteger(body, field) {
  const published = new RegExp(`"${field}":"([0-9]+)"`).exec(body);
  assert.notEqual(published, null, `the answer publishes no ${field}: ${body}`);
  return published[1];
}

/**
 * Ends everything this suite wrote: the signals about its rides, then the reservations themselves
 * through the service, and finally the rows. An invoice and an interval go with their rental, because
 * neither is a fact about anything else.
 */
async function endSuiteFinishes() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(`DELETE FROM outbox WHERE resource_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(
    `DELETE FROM outbox WHERE resource_id IN (
       SELECT invoice.id FROM invoices invoice
       JOIN rentals rental ON rental.id = invoice.rental_id
       WHERE rental.user_id IN ${mine})`,
  );
  await endSuiteReservations();
}
