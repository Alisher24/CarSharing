// The parts of the demonstration that only time and a real database can show: telemetry that
// genuinely goes stale, a reservation that genuinely expires, a seed that changes nothing on a
// second run, and a restoration that refuses rather than overwrite what a person did.
import assert from 'node:assert/strict';
import { before, describe, test } from 'node:test';
import { setTimeout as delay } from 'node:timers/promises';
import { call, registerAccount, resetRateLimits, sql, waitForReady } from './client.mjs';
import {
  holdTransaction,
  MAX_TELEMETRY_AGE_SECONDS,
  START_THRESHOLD_BASIS_POINTS,
  restoreScenario,
  runSeed,
  scalar,
  tryRestoreScenario,
  VEHICLES_PATH,
} from './fleet.mjs';
import { insertRental } from './rentalrows.mjs';

/** The rentals these cases write on behalf of a person, and remove again afterwards. */
const COMMITTED_PERSONAL_RENTAL = '01994342-6ba7-7000-8000-000900000001';
const CONCURRENT_PERSONAL_RENTAL = '01994342-6ba7-7000-8000-000900000002';
const FINISHED_PERSONAL_RENTAL = '01994342-6ba7-7000-8000-000900000003';

/** How long the concurrent case holds its transaction open before committing it. */
const HELD_SECONDS = 4;

/** Long enough for the restoration to have reached the lock the held transaction is on. */
const REACHES_THE_LOCK_MILLISECONDS = 1500;

/** Long enough for a position to pass the freshness limit when nothing is confirming it. */
const PAST_THE_LIMIT_MILLISECONDS = (MAX_TELEMETRY_AGE_SECONDS + 3) * 1000;

/** Long enough for the reservation sweep, which runs once a second, to have run. */
const SWEEP_MILLISECONDS = 4000;

async function readVehicle(id) {
  const response = await call(`${VEHICLES_PATH}/${id}`);
  assert.equal(response.status, 200, response.text);
  return response.json;
}

async function readCatalog() {
  const response = await call(VEHICLES_PATH);
  assert.equal(response.status, 200, response.text);
  return response.json.items;
}

/**
 * A vehicle the restoration puts back but no rental holds: the permanent exhausted example of its
 * powertrain. A vehicle left free for a person to book is not one the restoration owns, so a rental
 * of that one is not a conflict at all.
 */
function exhaustedScenarioVehicle() {
  return vehicleIdWhere(`
    id NOT IN (SELECT vehicle_id FROM rentals WHERE ended_at IS NULL)
    AND id IN (
      SELECT vehicle_id FROM vehicle_energy_sources
      GROUP BY vehicle_id
      HAVING bool_and(remaining * 10000 / capacity < ${START_THRESHOLD_BASIS_POINTS})
    )`);
}

function vehicleIdWhere(condition) {
  const id = scalar(`SELECT id FROM vehicles WHERE ${condition} ORDER BY id LIMIT 1`);
  assert.ok(id, `the scenario has no vehicle where ${condition}`);
  return id;
}

before(async () => {
  await waitForReady();
  runSeed();
  restoreScenario();
});

describe('telemetry the demonstration source keeps confirming', () => {
  test('a reporting vehicle stays fresh for longer than the freshness limit', async () => {
    const reporting = vehicleIdWhere('reporting');
    assert.equal((await readVehicle(reporting)).telemetry_status, 'fresh');
    await delay(PAST_THE_LIMIT_MILLISECONDS);
    assert.equal((await readVehicle(reporting)).telemetry_status, 'fresh');
  });

  test('a linked vehicle nothing confirms is stale, and an unlinked one is offline', async () => {
    const silent = vehicleIdWhere('connected AND NOT reporting');
    const unlinked = vehicleIdWhere('NOT connected');
    assert.equal((await readVehicle(silent)).telemetry_status, 'stale');
    assert.equal((await readVehicle(unlinked)).telemetry_status, 'offline');
  });

  test('an unlinked vehicle reports the technical reason beside its exhausted reserve', async () => {
    const unlinked = await readVehicle(vehicleIdWhere('NOT connected'));
    assert.equal(unlinked.status, 'unavailable');
    assert.deepEqual(unlinked.unavailable_reasons.slice().sort(), ['insufficient_energy', 'technical_unavailable']);
  });

  test('reading the catalog does not confirm a position', async () => {
    const silent = vehicleIdWhere('connected AND NOT reporting');
    const before = scalar(`SELECT confirmed_at FROM vehicle_telemetry WHERE vehicle_id = '${silent}'`);
    await readVehicle(silent);
    await readCatalog();
    assert.equal(scalar(`SELECT confirmed_at FROM vehicle_telemetry WHERE vehicle_id = '${silent}'`), before);
  });

  test('stopping the source for a vehicle makes its position genuinely go stale', async () => {
    const reporting = vehicleIdWhere('reporting');
    sql(`UPDATE vehicles SET reporting = false WHERE id = '${reporting}'`);
    try {
      await delay(PAST_THE_LIMIT_MILLISECONDS);
      assert.equal((await readVehicle(reporting)).telemetry_status, 'stale');
    } finally {
      sql(`UPDATE vehicles SET reporting = true WHERE id = '${reporting}'`);
    }
  });
});

describe('a reservation that runs out', () => {
  test('stands while its deadline is ahead', async () => {
    const rental = reservedRental();
    sql(`UPDATE rentals SET expires_at = now() + interval '1 hour' WHERE id = '${rental.id}'`);
    await delay(SWEEP_MILLISECONDS);
    assert.equal(stageOf(rental.id), 'reserved');
    assert.equal((await readVehicle(rental.vehicleId)).status, 'reserved');
  });

  test('ends at its own deadline rather than at the moment it was noticed', async () => {
    const rental = reservedRental();
    sql(`UPDATE rentals SET expires_at = now() - interval '1 hour' WHERE id = '${rental.id}'`);
    await delay(SWEEP_MILLISECONDS);

    assert.equal(stageOf(rental.id), 'expired');
    assert.equal(scalar(`SELECT ended_at = expires_at FROM rentals WHERE id = '${rental.id}'`), 't');
  });

  test('frees its vehicle in the catalog once nothing else holds it back', async () => {
    const rental = reservedRental();
    sql(`UPDATE rentals SET expires_at = now() WHERE id = '${rental.id}'`);
    await delay(SWEEP_MILLISECONDS);

    assert.equal(stageOf(rental.id), 'expired');
    assert.equal((await readVehicle(rental.vehicleId)).status, 'available');
  });

  test('leaves an unfit vehicle unavailable rather than available', async () => {
    const rental = reservedRental();
    sql(`UPDATE vehicle_energy_sources SET remaining = 0 WHERE vehicle_id = '${rental.vehicleId}'`);
    sql(`UPDATE rentals SET expires_at = now() WHERE id = '${rental.id}'`);
    await delay(SWEEP_MILLISECONDS);

    const released = await readVehicle(rental.vehicleId);
    assert.equal(released.status, 'unavailable');
    assert.ok(released.unavailable_reasons.includes('insufficient_energy'));
  });

  test('is expired exactly once, however many sweeps see it', async () => {
    const rental = reservedRental();
    sql(`UPDATE rentals SET expires_at = now() WHERE id = '${rental.id}'`);
    await delay(SWEEP_MILLISECONDS);
    const endedAt = scalar(`SELECT ended_at FROM rentals WHERE id = '${rental.id}'`);
    await delay(SWEEP_MILLISECONDS);

    assert.equal(scalar(`SELECT ended_at FROM rentals WHERE id = '${rental.id}'`), endedAt);
    assert.equal(scalar(`SELECT count(*) FROM rentals WHERE id = '${rental.id}'`), '1');
  });
});

describe('seeding and restoring the scenario', () => {
  test('a second seed adds nothing and changes nothing', () => {
    restoreScenario();
    const fleet = () => sql(`SELECT id, model, connected, reporting FROM vehicles ORDER BY id`);
    const reserves = () => sql(`SELECT vehicle_id, source_kind, remaining FROM vehicle_energy_sources ORDER BY 1, 2`);
    const deadlines = () => sql(`SELECT id, stage, expires_at FROM rentals ORDER BY id`);

    const before = { fleet: fleet(), reserves: reserves(), deadlines: deadlines() };
    runSeed();

    assert.equal(fleet(), before.fleet);
    assert.equal(reserves(), before.reserves);
    assert.equal(deadlines(), before.deadlines, 'a repeated seed moved a reservation deadline');
  });

  test('restoring puts back the reserves, the links and the prepared rentals', async () => {
    const rental = reservedRental();
    sql(`UPDATE vehicle_energy_sources SET remaining = 0 WHERE vehicle_id = '${rental.vehicleId}'`);
    sql(`UPDATE rentals SET expires_at = now() WHERE id = '${rental.id}'`);
    await delay(SWEEP_MILLISECONDS);
    assert.equal(stageOf(rental.id), 'expired');

    restoreScenario();

    assert.equal(stageOf(rental.id), 'reserved');
    assert.equal((await readVehicle(rental.vehicleId)).status, 'reserved');
    assert.equal(scalar(`SELECT expires_at > now() FROM rentals WHERE id = '${rental.id}'`), 't');
  });

  test('restoring puts the stale and offline examples back already stale', async () => {
    restoreScenario();
    const silent = vehicleIdWhere('connected AND NOT reporting');
    const unlinked = vehicleIdWhere('NOT connected');

    assert.equal((await readVehicle(silent)).telemetry_status, 'stale');
    assert.equal((await readVehicle(unlinked)).telemetry_status, 'offline');
  });

  test('restoring leaves every public state and every exhausted example in place', async () => {
    restoreScenario();
    const catalog = await readCatalog();
    for (const status of ['available', 'reserved', 'in_trip', 'unavailable']) {
      assert.ok(
        catalog.some((vehicle) => vehicle.status === status),
        status,
      );
    }
    const exhausted = new Set(
      catalog
        .filter(
          (vehicle) => vehicle.status === 'unavailable' && vehicle.unavailable_reasons.includes('insufficient_energy'),
        )
        .map((vehicle) => vehicle.powertrain_type),
    );
    assert.equal(exhausted.size, 5, `only ${[...exhausted].join(', ')} keep an exhausted example`);
  });

  test('a vehicle left for a person to book by hand is never touched', () => {
    restoreScenario();
    const bookable = vehicleIdWhere(`id NOT IN (SELECT vehicle_id FROM rentals)`);
    const reserves = () => sql(`SELECT remaining FROM vehicle_energy_sources WHERE vehicle_id = '${bookable}'`);

    sql(`UPDATE vehicle_energy_sources SET remaining = capacity WHERE vehicle_id = '${bookable}'`);
    const filled = reserves();
    restoreScenario();

    assert.equal(reserves(), filled, 'the restoration changed a vehicle it does not own');
  });
});

describe('restoring beside a person who rented a scenario vehicle', () => {
  test('refuses the whole restoration and changes nothing', async () => {
    restoreScenario();
    resetRateLimits();
    const { email } = await registerAccount('scenario-conflict');
    const taken = exhaustedScenarioVehicle();
    sql(personalRental(COMMITTED_PERSONAL_RENTAL, email, taken));

    const reserves = () => sql(`SELECT remaining FROM vehicle_energy_sources ORDER BY vehicle_id, source_kind`);
    const before = reserves();
    try {
      const refusal = tryRestoreScenario();
      assert.equal(refusal.ok, false, 'the restoration went ahead over a rental of a person');
      assert.match(refusal.output, /restoration refused/);
      assert.match(refusal.output, new RegExp(taken));
      assert.equal(reserves(), before, 'a refused restoration still wrote something');
      assert.equal(
        scalar(`SELECT count(*) FROM rentals WHERE user_id = (SELECT id FROM users WHERE email = '${email}')`),
        '1',
      );
    } finally {
      sql(`DELETE FROM rentals WHERE id = '${COMMITTED_PERSONAL_RENTAL}'`);
    }
  });

  test('succeeds again once that rental is gone', () => {
    const restored = tryRestoreScenario();
    assert.equal(restored.ok, true, restored.output);
  });

  // A rental created while the restoration is already running must not be overwritten either. The
  // restoration takes the scenario vehicles for update before it reads anything, so it waits for
  // the transaction that is creating the rental and then finds it.
  test('waits for a rental being created at the same moment, and then refuses', async () => {
    restoreScenario();
    resetRateLimits();
    const { email } = await registerAccount('scenario-race');
    const taken = exhaustedScenarioVehicle();

    const creating = holdTransaction(personalRental(CONCURRENT_PERSONAL_RENTAL, email, taken), HELD_SECONDS);
    await delay(REACHES_THE_LOCK_MILLISECONDS);
    const refusal = tryRestoreScenario();
    await creating;

    try {
      assert.equal(refusal.ok, false, 'the restoration overtook a rental being created');
      assert.equal(scalar(`SELECT count(*) FROM rentals WHERE id = '${CONCURRENT_PERSONAL_RENTAL}'`), '1');
    } finally {
      sql(`DELETE FROM rentals WHERE id = '${CONCURRENT_PERSONAL_RENTAL}'`);
    }
  });

  // A rental a person has already finished is their own history. The restoration refuses for it
  // exactly as it does for a live one, rather than deleting a row the scenario never wrote.
  test('refuses over a rental a person has already finished', async () => {
    restoreScenario();
    resetRateLimits();
    const { email } = await registerAccount('scenario-history');
    const taken = exhaustedScenarioVehicle();
    sql(finishedPersonalRental(FINISHED_PERSONAL_RENTAL, email, taken));

    const reserves = () => sql(`SELECT remaining FROM vehicle_energy_sources ORDER BY vehicle_id, source_kind`);
    const before = reserves();
    try {
      const refusal = tryRestoreScenario();
      assert.equal(refusal.ok, false, 'the restoration overwrote a rental a person had finished');
      assert.match(refusal.output, /restoration refused/);
      assert.match(refusal.output, new RegExp(taken));
      assert.equal(reserves(), before, 'a refused restoration still wrote something');
      assert.equal(
        scalar(`SELECT count(*) FROM rentals WHERE id = '${FINISHED_PERSONAL_RENTAL}' AND stage = 'completed'`),
        '1',
        'the rental a person finished is gone',
      );
    } finally {
      sql(`DELETE FROM rentals WHERE id = '${FINISHED_PERSONAL_RENTAL}'`);
    }
  });
});

/** The prepared reservation the expiry cases work on, named by the rental and the vehicle it holds. */
function reservedRental() {
  restoreScenario();
  const row = sql(`SELECT id || ' ' || vehicle_id FROM rentals WHERE stage = 'reserved' ORDER BY id LIMIT 1`);
  const [id, vehicleId] = row.split(' ');
  assert.ok(id && vehicleId, 'the scenario has no reservation');
  return { id, vehicleId };
}

function stageOf(rentalId) {
  return scalar(`SELECT stage FROM rentals WHERE id = '${rentalId}'`);
}

/**
 * The statement that gives a person a rental of a scenario vehicle. Reserving through the API
 * belongs to a later task, so the row is written directly; what matters here is that the
 * restoration finds a rental it does not own and stops.
 */
function personalRental(rentalId, email, vehicleId) {
  return insertRental({
    id: rentalId,
    email,
    vehicleId,
    stage: 'reserved',
    reservedAt: 'now()',
    expiresAt: `now() + interval '15 minutes'`,
  });
}

/**
 * The statement that gives a person a rental of a scenario vehicle that has already ended. It is
 * written directly for the same reason a live one is: what matters here is that the restoration
 * finds a rental it does not own, and leaves it as it stands.
 */
function finishedPersonalRental(rentalId, email, vehicleId) {
  return insertRental({
    id: rentalId,
    email,
    vehicleId,
    stage: 'completed',
    reservedAt: `now() - interval '30 minutes'`,
    expiresAt: `now() - interval '16 minutes'`,
    startedAt: `now() - interval '29 minutes'`,
    endedAt: `now() - interval '2 minutes'`,
    completionReason: 'user_finished',
  });
}
