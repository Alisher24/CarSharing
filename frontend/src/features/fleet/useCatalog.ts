import {
  fetchFleet,
  fetchServiceZones,
  fetchTariffs,
  type FleetSnapshot,
  type Tariff,
  type Vehicle,
  type Zone,
} from '../../shared/api/catalog';
import { loadedValue, type Resource } from '../../shared/api/Resource';
import { useResource, type ResourceHandle } from '../../shared/api/useResource';

/**
 * How often the fleet is read again. It is well inside the fifteen seconds a confirmed position
 * stays fresh, so a vehicle that goes stale, is released or is taken shows its new state promptly
 * rather than at the next thing a person happens to click.
 */
const FLEET_REFRESH_MILLISECONDS = 4000;

/**
 * Catalog is the three public resources, each loaded on its own. One of them failing leaves the
 * other two on screen, and each is retried by itself.
 */
export type Catalog = {
  fleet: ResourceHandle<FleetSnapshot>;
  zones: ResourceHandle<Zone[]>;
  tariffs: ResourceHandle<Tariff[]>;
};

export function useCatalog(): Catalog {
  return {
    fleet: useResource(fetchFleet, FLEET_REFRESH_MILLISECONDS),
    zones: useResource(fetchServiceZones),
    tariffs: useResource(fetchTariffs),
  };
}

/**
 * What a view found when it looked for the value it needs. The three ways of having nothing are
 * kept apart, because they are three different things to tell a person: the answer has not arrived
 * yet, the service answered and published none, or nothing could be read at all.
 */
export type Found<T> =
  { state: 'loading' } | { state: 'found'; value: T } | { state: 'none' } | { state: 'unreachable' };

function found<T>(resource: Resource<unknown>, value: T | undefined): Found<T> {
  if (value !== undefined) return { state: 'found', value };
  if (resource.phase === 'loading') return { state: 'loading' };
  if (resource.phase === 'failed') return { state: 'unreachable' };
  return { state: 'none' };
}

/** The vehicles last read, or none at all before the first reading arrives. */
export function catalogVehicles(catalog: Catalog): readonly Vehicle[] {
  return loadedValue(catalog.fleet.resource)?.vehicles ?? [];
}

/** What the map found when it looked for the service areas to draw. */
export function foundZones(catalog: Catalog): Found<readonly Zone[]> {
  const areas = loadedValue(catalog.zones.resource);
  return found(catalog.zones.resource, areas !== undefined && areas.length > 0 ? areas : undefined);
}

/**
 * What a card found when it looked for the price list to quote. The operator publishes one
 * demonstration tariff; nothing here substitutes a price of zero for a tariff it could not read.
 */
export function foundTariff(catalog: Catalog): Found<Tariff> {
  return found(catalog.tariffs.resource, loadedValue(catalog.tariffs.resource)?.[0]);
}
