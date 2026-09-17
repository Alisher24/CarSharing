// The simulated fleet on the assembled stack: the model the simulator advances, the ending a ride
// whose sources run out owes, and the demonstration control that prepares it.
//
// The internal operations are reached through the client containers rather than from the host, because
// the proxy publishes neither of them. A check that needs the fleet to stand still stops the simulator;
// the rest let it run and wait for the model, which advances on its own second.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import { compose, sql } from '../service.mjs';
import { waitForReady } from './client.mjs';
import {
  FIRST_VEHICLE,
  attemptsOwed,
  completionsOf,
  completionNotification,
  demandedOutcome,
  demoCommand,
  invoicesOf,
  issuedAt,
  serviceRequired,
  startSimulator,
  stopSimulator,
  storedCharge,
  storedEnding,
  storedModel,
  tickOnce,
  until,
} from './simulation.mjs';
import {
  IDEMPOTENCY_HEADER,
  availableVehicle,
  call,
  currentOf,
  endSuiteReservations,
  newAccount,
  newCommandKey,
  publishedVehicle,
  reserve,
  restoreScenario,
  storedRental,
} from './reservations.mjs';

/** The accounts this suite registers, which is also how it recognizes its own rows afterwards. */
const ACCOUNT_PREFIX = 'simulation';

/** Where a ride is started, and where the invoice of one is read. */
const startPath = (rentalId) => `/api/v1/reservations/${rentalId}/start`;
const invoicePath = (invoiceId) => `/api/v1/me/invoices/${invoiceId}`;
const pausePath = (rentalId) => `/api/v1/rides/${rentalId}/pause`;

/** The moment the contract publishes, which every answered moment is compared against. */
const MOMENT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;

/** The rates the demonstration charges for a started minute of each mode, which every invoice states. */
const DRIVING_RATE_TYIYN = 1234;
const PAUSED_RATE_TYIYN = 321;

/**
 * What a source is drained to when a check wants a ride to run out while it watches. A whole
 * demonstration battery lasts an hour of movement, so this many watt-hours are spent in about a
 * second: the ending is reached by the model rather than by waiting minutes for it.
 */
const DRAINED_TO = '20';

/**
 * What a source is filled to when a check wants the vehicle to keep moving through several ticks. It
 * is several minutes of movement, so a check that drives a vehicle through two ticks of its own finds
 * it still riding whatever the container start-up between them costs.
 */
const KEEPS_MOVING = '10000';

/** The source every vehicle of the demonstration's first group carries. */
const BATTERY = 'battery';

/** The vehicles this suite drove, so it can put them back into service before it finishes. */
const vehiclesUsed = new Set();

before(async () => {
  await waitForReady();
  // The simulator is a deliberate part of the demonstration rather than something a running stack
  // starts, so the suite that checks the moving fleet starts it and stops it again.
  compose('--profile', 'demo', 'up', '--detach', 'simulator');
});

// Every check drives a vehicle the next one would otherwise find drained and out of service, and the
// fleet is advanced by the clock the checks that stop it put back.
beforeEach(async () => {
  await endSuiteSimulation();
  startSimulator();
});

after(async () => {
  await endSuiteSimulation();
  restoreScenario();
  compose('--profile', 'demo', 'stop', 'simulator');
});

describe('the simulated fleet', () => {
  test('advances the fleet once per identifier and reproduces what it answered', async () => {
    // Only the ticks this check sends advance the fleet, so what each one changed is what it did
    // rather than what the clock did beside it.
    stopSimulator();
    const { vehicleId, rentalId } = await riding();
    await until(() => storedModel(vehicleId) !== undefined, 'the model never met the vehicle');
    await demoCommand('set-energy-remaining', '--vehicle', vehicleId, '--source', BATTERY, '--remaining', KEEPS_MOVING);

    const first = tickOnce(newCommandKey());
    assert.match(first.processed_at, MOMENT);
    const afterFirst = storedModel(vehicleId);
    const spentAfterFirst = BigInt(storedCharge(vehicleId, BATTERY));

    // The same identifier is the same call: it answers what the first one answered and advances
    // nothing, which is what makes a tick whose answer was lost cost no second movement.
    const repeated = tickOnce(first.tick_id);
    assert.equal(repeated.tick_id, first.tick_id);
    assert.equal(repeated.processed_at, first.processed_at, 'a repeated tick advanced the fleet');
    assert.equal(storedModel(vehicleId).processedAt, afterFirst.processedAt);
    assert.equal(
      BigInt(storedCharge(vehicleId, BATTERY)),
      spentAfterFirst,
      'a repeated tick spent the reserve a second time',
    );

    // A new identifier is a new call, and it advances the model past the moment the first one reached.
    const next = tickOnce(newCommandKey());
    assert.ok(next.processed_at > first.processed_at, 'a later tick did not advance the fleet');
    assert.equal(storedModel(vehicleId).processedAt, next.processed_at);
    assert.ok(
      BigInt(storedCharge(vehicleId, BATTERY)) > spentAfterFirst,
      'the vehicle moved through two ticks without spending anything',
    );
    assert.equal(storedRental(rentalId)[0], 'active', 'the ride ended while it still had a reserve');

    startSimulator();
  });

  test('ends a ride whose sources run out, at the moment they did', async () => {
    const { account, vehicleId, rentalId } = await riding();
    await until(() => storedModel(vehicleId) !== undefined, 'the model never met the vehicle');
    await demoCommand('set-energy-remaining', '--vehicle', vehicleId, '--source', BATTERY, '--remaining', DRAINED_TO);

    const ending = await until(() => {
      const stored = storedEnding(rentalId);
      return stored?.reason === 'energy_depleted' ? stored : undefined;
    }, 'the ride whose sources ran out was never ended');
    assert.match(ending.endedAt, MOMENT);
    assert.deepEqual(ending.sources, [BATTERY]);
    assert.equal(invoicesOf(rentalId), 1, 'the ending did not issue exactly one invoice');
    assert.equal(completionsOf(rentalId), 1, 'the ending did not report itself exactly once');

    // The report tells the account the moment the ride ended rather than the moment the service
    // noticed, and the invoice is priced to that same moment.
    const notification = completionNotification(rentalId);
    assert.equal(notification.endedAt, ending.endedAt);
    assert.equal(notification.reason, 'energy_depleted');
    assert.deepEqual(notification.sources, [BATTERY]);
    assert.ok(issuedAt(rentalId) >= ending.endedAt, 'the invoice was issued before the ride it prices had ended');

    // The ride is over as far as the account is concerned, and the result is reachable from the
    // invoice the report names.
    assert.equal((await currentOf(account)).json.kind, 'none');
    const invoice = await call(invoicePath(notification.invoiceId), { cookie: account.cookie });
    assert.equal(invoice.status, 200, invoice.text);
    assert.equal(invoice.json.invoice.completion.reason, 'energy_depleted');
    assert.deepEqual(invoice.json.invoice.completion.exhausted_sources, [BATTERY]);
    assert.equal(invoice.json.invoice.total_amount_tyiyn, String(DRIVING_RATE_TYIYN));
    assert.equal(attemptsOwed(notification.invoiceId), 1, 'the first payment attempt is not owed');

    // The vehicle is out of service, and the catalog says so rather than publishing it as free.
    assert.equal(serviceRequired(vehicleId), true);
    const published = await publishedVehicle(vehicleId);
    assert.equal(published.status, 'unavailable');
    assert.ok(published.unavailable_reasons.includes('service_required'));

    // Filling it does not put it back: the ride ended because the reserves ran out, and only
    // servicing answers that.
    await demoCommand('set-energy-remaining', '--vehicle', vehicleId, '--source', BATTERY, '--remaining', '1000');
    assert.equal(serviceRequired(vehicleId), true, 'a refill cleared the service requirement');
    const refilled = await publishedVehicle(vehicleId);
    assert.equal(refilled.status, 'unavailable');
    assert.ok(refilled.unavailable_reasons.includes('service_required'));

    await demoCommand('mark-serviced', '--vehicle', vehicleId);
    assert.equal(serviceRequired(vehicleId), false);
    assert.equal((await publishedVehicle(vehicleId)).status, 'available');
    assert.equal(storedModel(vehicleId).depleted, false);
  });

  // The ride is on pause while its sources run out. A paused ride holds one open interval of the
  // paused mode, so the ending has to close it at the moment of the exhaustion rather than at the
  // moment the service noticed, and the invoice has to price the paused minutes alone.
  test('ends a paused ride at the moment its sources ran out', async () => {
    const { account, vehicleId, rentalId } = await riding();
    const paused = await rideCommand('pause', rentalId, account);
    assert.equal(paused.status, 200, paused.text);
    assert.equal((await currentOf(account)).json.rental.state, 'paused');
    await until(() => storedModel(vehicleId) !== undefined, 'the model never met the vehicle');
    await demoCommand('set-energy-remaining', '--vehicle', vehicleId, '--source', BATTERY, '--remaining', DRAINED_TO);

    const ending = await until(() => {
      const stored = storedEnding(rentalId);
      return stored?.reason === 'energy_depleted' ? stored : undefined;
    }, 'the paused ride whose sources ran out was never ended');
    assert.deepEqual(ending.sources, [BATTERY]);
    assert.equal(invoicesOf(rentalId), 1, 'the ending did not issue exactly one invoice');
    assert.equal(completionsOf(rentalId), 1, 'the ending did not report itself exactly once');

    // The interval of the pause was closed by the moment of the exhaustion: a paused ride is exactly
    // one open interval, and the ending is what closes it.
    const openIntervals = sql(
      `SELECT count(*) FROM ride_segments WHERE rental_id = '${rentalId}' AND ended_at IS NULL`,
    );
    assert.equal(openIntervals, '0', 'the ending left an interval of the ride open');
    // Every interval is closed, and the ride ended in the mode it was holding: the last interval is
    // the pause, which is what the ending closed.
    const lastMode = sql(
      `SELECT mode FROM ride_segments WHERE rental_id = '${rentalId}' ORDER BY started_at DESC LIMIT 1`,
    );
    assert.equal(lastMode, 'paused', 'the ride did not end in the mode it was holding');
    const pausedIntervals = sql(
      `SELECT count(*) FROM ride_segments WHERE rental_id = '${rentalId}' AND mode = 'paused'`,
    );
    assert.ok(Number(pausedIntervals) >= 1, 'the ride of this check recorded no pause at all');
    // The pause outlasted the driving that preceded it: this check pauses the ride and drains it while
    // it stands, so a bill for driving would be a bill for time the vehicle did not move.
    const pausedDuration = sql(
      `SELECT sum(extract(epoch FROM (ended_at - started_at)))::bigint FROM ride_segments
       WHERE rental_id = '${rentalId}' AND mode = 'paused'`,
    );
    const drivingDuration = sql(
      `SELECT coalesce(sum(extract(epoch FROM (ended_at - started_at)))::bigint, 0) FROM ride_segments
       WHERE rental_id = '${rentalId}' AND mode = 'driving'`,
    );
    assert.ok(
      Number(pausedDuration) >= Number(drivingDuration),
      `the paused interval was shorter than the driving one: ${pausedDuration} against ${drivingDuration}`,
    );

    // The invoice prices the paused mode alone, to the same moment the ride ended.
    const notification = completionNotification(rentalId);
    assert.equal(notification.endedAt, ending.endedAt);
    assert.equal(notification.reason, 'energy_depleted');
    const invoice = await call(invoicePath(notification.invoiceId), { cookie: account.cookie });
    assert.equal(invoice.status, 200, invoice.text);
    assert.equal(invoice.json.invoice.completion.reason, 'energy_depleted');
    assert.deepEqual(invoice.json.invoice.completion.exhausted_sources, [BATTERY]);
    // Both modes are priced at their own rate on the started minutes each recorded, and the total is
    // the sum of the two lines rather than a figure of its own.
    const driving = invoice.json.invoice.lines.find((line) => line.mode === 'driving');
    const pausedLine = invoice.json.invoice.lines.find((line) => line.mode === 'paused');
    assert.equal(driving.rate_tyiyn_per_started_minute, String(DRIVING_RATE_TYIYN), invoice.text);
    assert.equal(pausedLine.rate_tyiyn_per_started_minute, String(PAUSED_RATE_TYIYN), invoice.text);
    assert.equal(
      driving.amount_tyiyn,
      String(Number(driving.billed_started_minutes) * DRIVING_RATE_TYIYN),
      `the driving line is not its minutes at its rate: ${invoice.text}`,
    );
    assert.equal(
      pausedLine.amount_tyiyn,
      String(Number(pausedLine.billed_started_minutes) * PAUSED_RATE_TYIYN),
      `the paused line is not its minutes at its rate: ${invoice.text}`,
    );
    assert.ok(
      Number(pausedLine.billed_started_minutes) >= 1,
      `the invoice billed no paused minute at all: ${invoice.text}`,
    );
    assert.equal(
      invoice.json.invoice.total_amount_tyiyn,
      String(Number(driving.amount_tyiyn) + Number(pausedLine.amount_tyiyn)),
      `the total is not the sum of the lines: ${invoice.text}`,
    );
    assert.equal(attemptsOwed(notification.invoiceId), 1, 'the first payment attempt is not owed');

    // The vehicle is out of service, as after any exhaustion of its sources.
    assert.equal(serviceRequired(vehicleId), true);
    await demoCommand('mark-serviced', '--vehicle', vehicleId);
  });

  test('ends a ride at the read when the simulator is stopped', async () => {
    stopSimulator();
    const { account, vehicleId, rentalId } = await riding();
    await until(() => storedModel(vehicleId) !== undefined, 'the model never met the vehicle');
    await demoCommand('set-energy-remaining', '--vehicle', vehicleId, '--source', BATTERY, '--remaining', DRAINED_TO);

    // Nothing advances the fleet, so nothing has noticed that the vehicle has run out.
    const standing = storedModel(vehicleId);
    await new Promise((resolve) => setTimeout(resolve, 2_000));
    assert.equal(storedModel(vehicleId).processedAt, standing.processedAt);
    assert.equal(storedEnding(rentalId), undefined, 'a stopped simulator ended a ride');

    // The read reconciles the model with the clock under the locks it already holds, and the ending is
    // written before the answer: the account is told there is nothing current, which is true.
    const snapshot = await currentOf(account);
    assert.equal(snapshot.status, 200, snapshot.text);
    assert.equal(snapshot.json.kind, 'none');
    assert.equal(storedEnding(rentalId).reason, 'energy_depleted');
    assert.equal(invoicesOf(rentalId), 1);
    assert.equal(serviceRequired(vehicleId), true);

    startSimulator();
  });

  test('prices a late finding to the moment the ride ran out', async () => {
    stopSimulator();
    const { account, vehicleId, rentalId } = await riding();
    await until(() => storedModel(vehicleId) !== undefined, 'the model never met the vehicle');
    await demoCommand('set-energy-remaining', '--vehicle', vehicleId, '--source', BATTERY, '--remaining', DRAINED_TO);

    // The ride runs out while nothing is looking, and the command that finds it arrives seconds
    // later. What it is answered with is the ending that happened rather than its own moment.
    await new Promise((resolve) => setTimeout(resolve, 3_000));
    const found = await currentOf(account);
    assert.equal(found.json.kind, 'none');

    const ending = storedEnding(rentalId);
    assert.ok(
      issuedAt(rentalId) > ending.endedAt,
      'the invoice was issued at the moment the ride ended rather than at the moment it was found',
    );
    assert.equal(invoicesOf(rentalId), 1);
    assert.equal(completionsOf(rentalId), 1);
    assert.equal(serviceRequired(vehicleId), true);

    startSimulator();
  });

  test('reproduces a repeated demonstration command and refuses a changed payload', async () => {
    const { rentalId, vehicleId } = await riding();
    const actionId = newCommandKey();

    const first = demoCommand(
      'set-next-payment-outcome',
      '--rental',
      rentalId,
      '--outcome',
      'failed',
      '--action-id',
      actionId,
    );
    assert.equal(first.action_id, actionId);
    assert.match(first.server_time, MOMENT);
    assert.equal(demandedOutcome(rentalId), 'failed');

    const repeated = demoCommand(
      'set-next-payment-outcome',
      '--rental',
      rentalId,
      '--outcome',
      'failed',
      '--action-id',
      actionId,
    );
    assert.deepEqual(repeated, first, 'a repeated action answered something else');

    // The same identifier with another payload is a conflict rather than a repeat, and the refusal is
    // what the client is told.
    const changed = runDemoCommand(
      'set-next-payment-outcome',
      '--rental',
      rentalId,
      '--outcome',
      'paid',
      '--action-id',
      actionId,
    );
    assert.equal(changed.ok, false, 'a changed payload was accepted under a used identifier');
    assert.match(changed.output, /IDEMPOTENCY_CONFLICT/);
    assert.equal(demandedOutcome(rentalId), 'failed', 'a refused action changed the demand');

    // A command that names a vehicle a ride holds is refused while the ride is moving, and the
    // vehicle stays where the ride put it.
    const moved = runDemoCommand('set-position', '--vehicle', vehicleId, '--longitude', '74.6', '--latitude', '42.87');
    assert.equal(moved.ok, false, 'a moving vehicle was moved by hand');
    assert.match(moved.output, /VEHICLE_IN_USE/);
  });

  test('does not end a ride twice when two simulators run at once', async () => {
    compose('up', '--detach', '--scale', 'simulator=2', 'simulator');
    const { vehicleId, rentalId } = await riding();
    await until(() => storedModel(vehicleId) !== undefined, 'the model never met the vehicle');
    await demoCommand('set-energy-remaining', '--vehicle', vehicleId, '--source', BATTERY, '--remaining', DRAINED_TO);

    await until(
      () => storedEnding(rentalId)?.reason === 'energy_depleted',
      'the ride whose sources ran out was never ended',
    );
    // Both copies advance the one fleet under the shared lock order, so the second waits for the first
    // and finds the ending already written: one invoice and one report, whatever the two of them did.
    await new Promise((resolve) => setTimeout(resolve, 2_000));
    assert.equal(invoicesOf(rentalId), 1, 'a second simulator issued a second invoice');
    assert.equal(completionsOf(rentalId), 1, 'a second simulator reported the ending again');

    compose('up', '--detach', '--scale', 'simulator=1', 'simulator');
  });
});

/**
 * One account holding a started ride. The vehicle is taken from the catalogue rather than named, so a
 * check never depends on what another suite left standing.
 */
async function riding() {
  const account = await newAccount(ACCOUNT_PREFIX);
  const vehicleId = await freeVehicle();
  const reserved = await reserve(vehicleId, newCommandKey(), account);
  assert.equal(reserved.status, 201, reserved.text);
  const rentalId = reserved.json.rental.id;

  const started = await call(startPath(rentalId), {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
  });
  assert.equal(started.status, 200, started.text);
  vehiclesUsed.add(vehicleId);
  return { account, vehicleId, rentalId };
}

/** Sends one ride command for one rental, which the checks above use to reach a ride in progress. */
function rideCommand(operation, rentalId, account) {
  const path = { pause: pausePath(rentalId), resume: `/api/v1/rides/${rentalId}/resume` }[operation];
  return call(path, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
  });
}

/**
 * One vehicle free to take: the first group's vehicle while the demonstration has left it free, which
 * is the one whose single source the checks above drain, and any free vehicle otherwise.
 */
async function freeVehicle() {
  const published = await publishedVehicle(FIRST_VEHICLE).catch(() => undefined);
  if (published?.status === 'available') {
    return FIRST_VEHICLE;
  }
  return availableVehicle();
}

/**
 * Ends everything this suite still holds and puts the vehicles it drove back into service, so the
 * suites that follow read the demonstration rather than what these checks did to it. The rides are
 * ended first, because a vehicle a live ride holds cannot be serviced.
 */
async function endSuiteSimulation() {
  await endSuiteReservations();
  for (const vehicleId of vehiclesUsed) {
    runDemoCommand('mark-serviced', '--vehicle', vehicleId);
  }
  vehiclesUsed.clear();
  sql('DELETE FROM demo_payment_outcomes');
}

/** Runs one demonstration command and reports whether it was accepted, with its output either way. */
function runDemoCommand(...arguments_) {
  try {
    const output = compose('--profile', 'demo', 'run', '--rm', 'democontrol', ...arguments_);
    return { ok: true, output };
  } catch (failure) {
    return { ok: false, output: `${failure.stdout ?? ''}${failure.stderr ?? ''}` };
  }
}
