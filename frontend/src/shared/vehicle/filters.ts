import type { PowertrainType, Vehicle } from '../api/catalog.ts';
import { POWERTRAIN_TYPES, STATUSES, type VehicleStatus } from '../vehicle/spell.ts';

/**
 * What a person has narrowed the fleet to. An empty group narrows nothing, so the initial view
 * shows every powertrain and every state.
 */
export type FleetFilters = {
  powertrainTypes: ReadonlySet<PowertrainType>;
  statuses: ReadonlySet<VehicleStatus>;
};

export const NO_FILTERS: FleetFilters = { powertrainTypes: new Set(), statuses: new Set() };

/** The statuses a person can choose between, which is every state the contract publishes. */
export const FILTERABLE_STATUSES: readonly VehicleStatus[] = STATUSES;

/**
 * matches applies the two groups the way a person reads them: any of the chosen powertrains, and
 * any of the chosen states. A vehicle must satisfy both groups to stay in the result.
 */
export function matches(vehicle: Vehicle, filters: FleetFilters): boolean {
  return admits(filters.powertrainTypes, vehicle.powertrain_type) && admits(filters.statuses, vehicle.status);
}

function admits<T>(chosen: ReadonlySet<T>, value: T): boolean {
  return chosen.size === 0 || chosen.has(value);
}

/**
 * selectVehicles is the one result the map and the list both draw, so a marker and a row can never
 * disagree about what is shown. The service answers in a stable order and filtering preserves it.
 */
export function selectVehicles(vehicles: readonly Vehicle[], filters: FleetFilters): Vehicle[] {
  return vehicles.filter((vehicle) => matches(vehicle, filters));
}

/** Whether the state filter is currently narrowed to exactly the vehicles a person could take. */
export function showsOnlyAvailable(filters: FleetFilters): boolean {
  return filters.statuses.size === 1 && filters.statuses.has('available');
}

/**
 * withOnlyAvailable is the prominent switch. It is the state filter rather than a second one
 * beside it, so turning it on selects that one state and turning it off widens back to all of them.
 */
export function withOnlyAvailable(filters: FleetFilters, only: boolean): FleetFilters {
  return { ...filters, statuses: only ? new Set<VehicleStatus>(['available']) : new Set() };
}

export function withPowertrainToggled(filters: FleetFilters, powertrain: PowertrainType): FleetFilters {
  return { ...filters, powertrainTypes: toggled(filters.powertrainTypes, powertrain) };
}

export function withStatusToggled(filters: FleetFilters, status: VehicleStatus): FleetFilters {
  return { ...filters, statuses: toggled(filters.statuses, status) };
}

function toggled<T>(chosen: ReadonlySet<T>, value: T): Set<T> {
  const next = new Set(chosen);
  if (!next.delete(value)) next.add(value);
  return next;
}

/** The powertrains a person can choose between, in the order they are offered. */
export const FILTERABLE_POWERTRAIN_TYPES: readonly PowertrainType[] = POWERTRAIN_TYPES;
