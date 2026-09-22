import type { FleetSnapshot, Vehicle } from '../../shared/api/catalog.ts';
import { versionIn } from '../../shared/read/observedResource.ts';
import { compareVersions, isNewerTimestamp, isNewerVersion } from '../../shared/read/version.ts';

/**
 * The fleet as one read answered it, together with the moment that answer was computed at. The
 * moment is the one the service stated, never the one the response happened to arrive at: a
 * snapshot describes the fleet at a time, and freshness is measured against that time.
 */
export type FleetReading = { vehicles: readonly Vehicle[]; serverTime: string };

/**
 * mergeFleetSnapshot folds one read into the snapshot already held. A vehicle whose version is
 * greater replaces the stored one whatever the arrival order; a vehicle whose version is lower is
 * discarded however late the answer is; an equal version keeps the stored object unless the read
 * was computed at a later moment, and such a read publishes a freshness the stored data cannot
 * state by itself.
 */
export function mergeFleetSnapshot(stored: FleetReading, incoming: FleetSnapshot): FleetReading {
  const later = isNewerTimestamp(incoming.serverTime, stored.serverTime);

  return {
    vehicles: mergeVehicles(stored.vehicles, incoming.vehicles, later),
    serverTime: later ? incoming.serverTime : stored.serverTime,
  };
}

/**
 * updatesFleetReading says whether a read is worth showing over the snapshot already held: it is
 * computed at a later moment, or it publishes a version the stored snapshot does not have.
 */
export function updatesFleetReading(stored: FleetReading, incoming: FleetSnapshot): boolean {
  if (isNewerTimestamp(incoming.serverTime, stored.serverTime)) return true;

  return incoming.vehicles.some((vehicle) =>
    isNewerVersion(versionOf(incoming, vehicle.id), versionOf(stored, vehicle.id)),
  );
}

/**
 * The version one reading published for a vehicle. A reading says what it holds two ways — as the
 * answer the service gave, and as the snapshot the interface keeps — and both are read by the rule
 * that states what the version of a vehicle that is not there is worth.
 */
export function versionOf(reading: { vehicles: readonly Vehicle[] }, id: string): string {
  return versionIn(reading.vehicles, id);
}

function mergeVehicles(stored: readonly Vehicle[], incoming: readonly Vehicle[], later: boolean): readonly Vehicle[] {
  const published = new Map(incoming.map((vehicle) => [vehicle.id, vehicle]));
  const merged = stored.map((vehicle) => acceptedVehicle(vehicle, published.get(vehicle.id), later));

  for (const vehicle of incoming) {
    if (!stored.some((held) => held.id === vehicle.id)) merged.push(vehicle);
  }

  return merged;
}

function acceptedVehicle(stored: Vehicle, published: Vehicle | undefined, later: boolean): Vehicle {
  if (published === undefined) return stored;

  const order = compareVersions(published.version, stored.version);
  if (order > 0) return published;
  if (order < 0) return stored;
  return later ? published : stored;
}
