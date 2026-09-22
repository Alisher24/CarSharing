import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { PowertrainType, Vehicle } from '../api/catalog.ts';
import type { VehicleStatus } from './spell.ts';
import {
  NO_FILTERS,
  selectVehicles,
  showsOnlyAvailable,
  withOnlyAvailable,
  withPowertrainToggled,
  withStatusToggled,
} from './filters.ts';

/**
 * A vehicle stated only by what the filters read. Everything else the contract requires is absent,
 * because narrowing the fleet must depend on nothing else.
 */
function vehicle(id: string, powertrain: PowertrainType, status: VehicleStatus): Vehicle {
  return { id, powertrain_type: powertrain, status } as unknown as Vehicle;
}

const fleet: Vehicle[] = [
  vehicle('1', 'electric', 'available'),
  vehicle('2', 'electric', 'reserved'),
  vehicle('3', 'diesel', 'available'),
  vehicle('4', 'diesel', 'unavailable'),
  vehicle('5', 'hybrid', 'in_trip'),
];

function identifiers(vehicles: readonly Vehicle[]): string[] {
  return vehicles.map((selected) => selected.id);
}

describe('narrowing the fleet', () => {
  test('no chosen value shows the whole fleet', () => {
    assert.deepEqual(identifiers(selectVehicles(fleet, NO_FILTERS)), ['1', '2', '3', '4', '5']);
  });

  test('several values of one group are alternatives', () => {
    const twoPowertrains = withPowertrainToggled(withPowertrainToggled(NO_FILTERS, 'electric'), 'diesel');
    assert.deepEqual(identifiers(selectVehicles(fleet, twoPowertrains)), ['1', '2', '3', '4']);
  });

  test('two groups must both be satisfied', () => {
    const dieselAndAvailable = withStatusToggled(withPowertrainToggled(NO_FILTERS, 'diesel'), 'available');
    assert.deepEqual(identifiers(selectVehicles(fleet, dieselAndAvailable)), ['3']);
  });

  test('a filter nothing matches gives an empty result rather than the whole fleet', () => {
    const hybridAndAvailable = withStatusToggled(withPowertrainToggled(NO_FILTERS, 'hybrid'), 'available');
    assert.deepEqual(identifiers(selectVehicles(fleet, hybridAndAvailable)), []);
  });

  test('the order the service published is kept', () => {
    const everyStatus = ['unavailable', 'available', 'in_trip', 'reserved'] as const;
    const allChosen = everyStatus.reduce(withStatusToggled, NO_FILTERS);
    assert.deepEqual(identifiers(selectVehicles(fleet, allChosen)), ['1', '2', '3', '4', '5']);
  });
});

describe('the only-available switch', () => {
  test('turning it on narrows the state group to exactly that state', () => {
    const only = withOnlyAvailable(NO_FILTERS, true);
    assert.ok(showsOnlyAvailable(only));
    assert.deepEqual(identifiers(selectVehicles(fleet, only)), ['1', '3']);
  });

  test('turning it off widens the state group back to every state', () => {
    const widened = withOnlyAvailable(withOnlyAvailable(NO_FILTERS, true), false);
    assert.equal(showsOnlyAvailable(widened), false);
    assert.deepEqual(identifiers(selectVehicles(fleet, widened)), ['1', '2', '3', '4', '5']);
  });

  test('choosing a second state turns the switch off, because it is the state filter', () => {
    const alsoReserved = withStatusToggled(withOnlyAvailable(NO_FILTERS, true), 'reserved');
    assert.equal(showsOnlyAvailable(alsoReserved), false);
    assert.deepEqual(identifiers(selectVehicles(fleet, alsoReserved)), ['1', '2', '3']);
  });

  test('it leaves the powertrain group alone', () => {
    const electricOnlyAvailable = withOnlyAvailable(withPowertrainToggled(NO_FILTERS, 'electric'), true);
    assert.deepEqual(identifiers(selectVehicles(fleet, electricOnlyAvailable)), ['1']);
  });
});
