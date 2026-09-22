import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { FleetSnapshot, Vehicle } from '../../shared/api/catalog.ts';
import { vehicle } from '../../shared/vehicle/fixtures.ts';
import { mergeFleetSnapshot, updatesFleetReading, versionOf, type FleetReading } from './fleetSnapshot.ts';

const EARLIER = '2026-09-12T07:15:30.123456Z';
const LATER = '2026-09-12T07:16:00.000000Z';

/** A fleet snapshot as one read answered it: the moment it was computed at and the vehicles in it. */
function snapshot(serverTime: string, vehicles: readonly Vehicle[]): FleetSnapshot {
  return { serverTime, vehicles: [...vehicles] };
}

function reading(serverTime: string, vehicles: readonly ReturnType<typeof vehicle>[]): FleetReading {
  return { vehicles, serverTime };
}

describe('accepting a vehicle snapshot', () => {
  const stored = reading(EARLIER, [vehicle('v1', '42', 'Прежняя модель')]);

  test('a greater version is accepted', () => {
    const merged = mergeFleetSnapshot(stored, snapshot(LATER, [vehicle('v1', '43', 'Новая модель')]));

    assert.equal(merged.vehicles[0]?.model, 'Новая модель');
    assert.equal(merged.serverTime, LATER);
  });

  // The order answers arrive in is not the order they were computed in, so a lower version has to
  // be discarded whatever moment it happened to reach the browser.
  test('a lower version is discarded however late the answer is', () => {
    const merged = mergeFleetSnapshot(stored, snapshot(LATER, [vehicle('v1', '41', 'Устаревшая модель')]));

    assert.equal(merged.vehicles[0]?.model, 'Прежняя модель');
  });

  // Freshness is computed at the moment of the snapshot, so a snapshot taken later can show a
  // vehicle that changed without its stored data changing at all.
  test('an equal version computed later replaces the stored values', () => {
    const nowStale = vehicle('v1', '42', 'Та же модель');
    nowStale.telemetry_status = 'stale';
    const merged = mergeFleetSnapshot(stored, snapshot(LATER, [nowStale]));

    assert.equal(merged.vehicles[0]?.telemetry_status, 'stale');
    assert.equal(merged.serverTime, LATER);
  });

  test('an equal version computed no later keeps the stored object', () => {
    const storedObject = stored.vehicles[0];
    const merged = mergeFleetSnapshot(stored, snapshot(EARLIER, [vehicle('v1', '42', 'Та же модель')]));

    assert.equal(merged.vehicles[0], storedObject);
  });

  test('an earlier snapshot moment does not move the stored moment back', () => {
    const merged = mergeFleetSnapshot(reading(LATER, [vehicle('v1', '42')]), snapshot(EARLIER, [vehicle('v1', '43')]));

    assert.equal(merged.serverTime, LATER);
  });

  test('a vehicle the snapshot does not mention keeps the values already held', () => {
    const held = reading(EARLIER, [vehicle('v1', '42'), vehicle('v2', '7')]);
    const merged = mergeFleetSnapshot(held, snapshot(LATER, [vehicle('v1', '43')]));

    assert.deepEqual(
      merged.vehicles.map((held) => held.id),
      ['v1', 'v2'],
    );
    assert.equal(merged.vehicles[1]?.version, '7');
  });

  test('a vehicle the snapshot adds appears after the ones already held', () => {
    const merged = mergeFleetSnapshot(stored, snapshot(LATER, [vehicle('v2', '1')]));

    assert.deepEqual(
      merged.vehicles.map((held) => held.id),
      ['v1', 'v2'],
    );
  });
});

describe('deciding whether a snapshot says anything', () => {
  test('a later moment alone is worth showing', () => {
    assert.equal(
      updatesFleetReading(reading(EARLIER, [vehicle('v1', '42')]), snapshot(LATER, [vehicle('v1', '42')])),
      true,
    );
  });

  test('a greater version alone is worth showing', () => {
    assert.equal(
      updatesFleetReading(reading(EARLIER, [vehicle('v1', '42')]), snapshot(EARLIER, [vehicle('v1', '43')])),
      true,
    );
  });

  test('the same versions at the same moment say nothing new', () => {
    assert.equal(
      updatesFleetReading(reading(EARLIER, [vehicle('v1', '42')]), snapshot(EARLIER, [vehicle('v1', '42')])),
      false,
    );
  });

  test('the version a snapshot publishes for a vehicle it does not have is the one before all', () => {
    assert.equal(versionOf(snapshot(LATER, []), 'v1'), '0');
    assert.equal(versionOf(snapshot(LATER, [vehicle('v1', '42')]), 'v1'), '42');
  });
});
