// The public catalog, the service zone and the tariff, observed on the real HTTP boundary and in
// the PostGIS the running service reads. Every claim here is made about the assembled stack: the
// fleet these tests describe is the one a visitor loads in a browser.
import assert from 'node:assert/strict';
import { before, describe, test } from 'node:test';
import { call, sql, waitForReady } from './client.mjs';
import {
  countStates,
  DRIVING_RATE_TYIYN,
  everySource,
  FLEET_SIZE,
  PAUSED_RATE_TYIYN,
  pointAt,
  POWERTRAIN_TYPES,
  restoreScenario,
  runSeed,
  scalar,
  START_THRESHOLD_BASIS_POINTS,
  STATES_PER_POWERTRAIN,
  TARIFFS_PATH,
  VEHICLES_PATH,
  vehiclesOfPowertrain,
  ZONES_PATH,
} from './fleet.mjs';

/** Words a public answer must never contain, because none of them is anybody's business. */
const PRIVATE_WORDS = ['user', 'email', 'rental', 'invoice', 'route', 'history'];

async function readCatalog() {
  const response = await call(VEHICLES_PATH);
  assert.equal(response.status, 200, response.text);
  return response.json;
}

before(async () => {
  await waitForReady();
  runSeed();
});

describe('the demonstration fleet a visitor loads', () => {
  test('holds five vehicles of each of the five powertrains', async () => {
    const catalog = await readCatalog();
    assert.equal(catalog.items.length, FLEET_SIZE);
    for (const powertrain of POWERTRAIN_TYPES) {
      assert.equal(vehiclesOfPowertrain(catalog.items, powertrain).length, 5, powertrain);
    }
  });

  test('shows every public state within every powertrain', async () => {
    const catalog = await readCatalog();
    for (const powertrain of POWERTRAIN_TYPES) {
      const counted = countStates(vehiclesOfPowertrain(catalog.items, powertrain));
      assert.deepEqual(counted, STATES_PER_POWERTRAIN, powertrain);
    }
  });

  test('shows both a moving and a paused ride among the prepared trips', async () => {
    const catalog = await readCatalog();
    const modes = catalog.items.filter((vehicle) => vehicle.status === 'in_trip').map((one) => one.ride_mode);
    assert.ok(modes.includes('driving'), `ride modes were ${modes.join(', ')}`);
    assert.ok(modes.includes('paused'), `ride modes were ${modes.join(', ')}`);
  });

  test('keeps a stable order between two readings', async () => {
    const first = await readCatalog();
    const second = await readCatalog();
    assert.deepEqual(
      second.items.map((vehicle) => vehicle.id),
      first.items.map((vehicle) => vehicle.id),
    );
  });

  test('lists every energy source separately, never a total', async () => {
    const catalog = await readCatalog();
    const hybrids = vehiclesOfPowertrain(catalog.items, 'hybrid');
    for (const hybrid of hybrids) {
      const kinds = hybrid.energy_sources.map((source) => source.kind).sort();
      assert.deepEqual(kinds, ['battery', 'gasoline'], hybrid.model);
    }
  });
});

describe('what makes a vehicle fit to start', () => {
  test('a source is startable exactly from the threshold upwards', async () => {
    const catalog = await readCatalog();
    for (const source of everySource(catalog.items)) {
      assert.equal(
        source.can_start,
        source.remaining_basis_points >= START_THRESHOLD_BASIS_POINTS,
        `${source.kind} at ${source.remaining_basis_points}`,
      );
    }
  });

  test('the fleet actually contains the two reserves the rule turns on', async () => {
    const catalog = await readCatalog();
    const shares = everySource(catalog.items).map((source) => source.remaining_basis_points);
    assert.ok(shares.includes(START_THRESHOLD_BASIS_POINTS), 'no source sits exactly at the threshold');
    assert.ok(
      shares.includes(START_THRESHOLD_BASIS_POINTS - 1),
      'no source sits one ten-thousandth below the threshold',
    );
  });

  test('reserves of two sources are not added together', async () => {
    const catalog = await readCatalog();
    const notSummed = catalog.items.filter(
      (vehicle) =>
        vehicle.energy_sources.length > 1 &&
        vehicle.energy_sources.every((source) => !source.can_start) &&
        vehicle.energy_sources.reduce((total, source) => total + source.remaining_basis_points, 0) >=
          START_THRESHOLD_BASIS_POINTS,
    );
    assert.ok(notSummed.length > 0, 'the fleet has no vehicle whose reserves would pass if added');
    for (const vehicle of notSummed) {
      assert.equal(vehicle.status, 'unavailable', vehicle.model);
      assert.ok(vehicle.unavailable_reasons.includes('insufficient_energy'), vehicle.model);
    }
  });

  test('an empty battery does not hide a sufficient tank', async () => {
    const catalog = await readCatalog();
    const emptyBattery = vehiclesOfPowertrain(catalog.items, 'hybrid').filter((vehicle) =>
      vehicle.energy_sources.some((source) => source.kind === 'battery' && source.remaining_basis_points === 0),
    );
    assert.ok(emptyBattery.length > 0, 'the fleet has no hybrid with an empty battery');
    for (const vehicle of emptyBattery) {
      assert.equal(vehicle.status, 'available', vehicle.model);
    }
  });

  test('every powertrain keeps an exhausted example', async () => {
    const catalog = await readCatalog();
    for (const powertrain of POWERTRAIN_TYPES) {
      const exhausted = vehiclesOfPowertrain(catalog.items, powertrain).filter(
        (vehicle) => vehicle.status === 'unavailable' && vehicle.unavailable_reasons.includes('insufficient_energy'),
      );
      assert.ok(exhausted.length > 0, powertrain);
    }
  });
});

describe('a rental decides occupancy', () => {
  test('a vehicle a rental holds is occupied whatever its own condition says', async () => {
    const catalog = await readCatalog();
    const held = sql(`SELECT vehicle_id FROM rentals WHERE ended_at IS NULL ORDER BY vehicle_id`).split('\n');
    for (const vehicle of catalog.items) {
      const occupied = vehicle.status === 'reserved' || vehicle.status === 'in_trip';
      assert.equal(occupied, held.includes(vehicle.id), `${vehicle.model} is ${vehicle.status}`);
    }
  });

  // Draining a rented vehicle and reading the catalog again is the only way to see that the rental
  // still decides: a low reserve on a free vehicle would make it unavailable, and on a rented one
  // it must change nothing. The scenario is put back afterwards.
  test('a rented vehicle that runs out of energy is still shown as rented', async () => {
    const catalog = await readCatalog();
    const reserved = catalog.items.find((vehicle) => vehicle.status === 'reserved');
    assert.ok(reserved, 'the scenario has no reserved vehicle');

    sql(`UPDATE vehicle_energy_sources SET remaining = 0 WHERE vehicle_id = '${reserved.id}'`);
    try {
      const drained = await call(`${VEHICLES_PATH}/${reserved.id}`);
      assert.equal(drained.status, 200, drained.text);
      assert.equal(drained.json.status, 'reserved');
      assert.ok(drained.json.energy_sources.every((source) => !source.can_start));
    } finally {
      restoreScenario();
    }
  });
});

describe('what a public answer may say', () => {
  test('the catalog, the zone and the tariff answer a reader with no account', async () => {
    for (const path of [VEHICLES_PATH, ZONES_PATH, TARIFFS_PATH]) {
      const response = await call(path);
      assert.equal(response.status, 200, `${path}: ${response.text}`);
    }
  });

  test('nothing in the catalog names a renter, a route, an invoice or a history', async () => {
    const response = await call(VEHICLES_PATH);
    for (const word of PRIVATE_WORDS) {
      assert.ok(!response.text.toLowerCase().includes(word), `the catalog published ${word}`);
    }
  });

  test('one vehicle is readable on its own and an unknown one is not', async () => {
    const catalog = await readCatalog();
    const known = await call(`${VEHICLES_PATH}/${catalog.items[0].id}`);
    assert.equal(known.status, 200, known.text);
    assert.equal(known.json.id, catalog.items[0].id);

    const unknown = await call(`${VEHICLES_PATH}/01994342-6ba7-7000-8000-ffffffffffff`);
    assert.equal(unknown.status, 404, unknown.text);
    assert.equal(unknown.json.code, 'RESOURCE_NOT_FOUND');
  });
});

describe('the service zone in PostGIS', () => {
  test('is valid WGS84 geometry the database itself accepts', () => {
    assert.equal(scalar(`SELECT bool_and(ST_IsValid(area)) FROM service_zones`), 't');
    assert.equal(scalar(`SELECT bool_and(ST_SRID(area) = 4326) FROM service_zones`), 't');
  });

  test('covers a point inside it and a point on its boundary, but not one outside', () => {
    const covered = (point) => scalar(`SELECT bool_or(ST_Covers(area, ${point})) FROM service_zones`);
    assert.equal(covered(pointAt(74.6, 42.87)), 't', 'a point inside the zone');
    assert.equal(covered(pointAt(74.55, 42.87)), 't', 'a point on the western edge');
    assert.equal(covered(pointAt(74.55, 42.84)), 't', 'a corner of the zone');
    assert.equal(covered(pointAt(74.549999, 42.87)), 'f', 'a point just outside the western edge');
    assert.equal(covered(pointAt(42.87, 74.6)), 'f', 'a swapped coordinate pair');
  });

  test('holds every demonstration vehicle', () => {
    const outside = scalar(`
      SELECT count(*) FROM vehicle_telemetry telemetry
      WHERE NOT EXISTS (
        SELECT 1 FROM service_zones zone WHERE ST_Covers(zone.area, telemetry.position)
      )`);
    assert.equal(outside, '0');
  });

  test('reports a vehicle parked outside it as outside it, beside its other reasons', async () => {
    const catalog = await readCatalog();
    const exhausted = catalog.items.find(
      (vehicle) => vehicle.status === 'unavailable' && vehicle.unavailable_reasons.includes('insufficient_energy'),
    );
    assert.ok(exhausted, 'the scenario has no exhausted vehicle');

    // Just west of the western edge, which the boundary case above shows is not covered.
    sql(`
      UPDATE vehicle_telemetry
      SET position = ${pointAt(74.549999, 42.87)}
      WHERE vehicle_id = '${exhausted.id}'`);
    try {
      const moved = await call(`${VEHICLES_PATH}/${exhausted.id}`);
      assert.equal(moved.status, 200, moved.text);
      assert.equal(moved.json.status, 'unavailable');
      assert.ok(moved.json.unavailable_reasons.includes('outside_service_zone'), moved.text);
      assert.ok(moved.json.unavailable_reasons.includes('insufficient_energy'), moved.text);
    } finally {
      restoreScenario();
    }
  });

  test('is published longitude first, as the contract writes a coordinate', async () => {
    const response = await call(ZONES_PATH);
    assert.equal(response.status, 200, response.text);
    const [[first]] = response.json.items[0].geometry.coordinates;
    const [longitude, latitude] = first;
    assert.ok(longitude > 74 && longitude < 75, `longitude was ${longitude}`);
    assert.ok(latitude > 42 && latitude < 43, `latitude was ${latitude}`);
  });
});

describe('the demonstration tariff', () => {
  test('is published in whole tyiyn under the agreed policy', async () => {
    const response = await call(TARIFFS_PATH);
    assert.equal(response.status, 200, response.text);
    const [tariff] = response.json.items;
    assert.equal(tariff.currency, 'KGS');
    assert.equal(tariff.billing_policy, 'per_mode_started_minute_v1');
    assert.equal(tariff.driving_rate_tyiyn_per_started_minute, String(DRIVING_RATE_TYIYN));
    assert.equal(tariff.paused_rate_tyiyn_per_started_minute, String(PAUSED_RATE_TYIYN));
  });
});
