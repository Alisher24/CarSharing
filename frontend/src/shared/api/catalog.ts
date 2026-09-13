import { getTariffs, getVehicles, getZones } from './generated/sdk.gen';
import type { Tariff, Vehicle, Zone } from './generated/types.gen';

export type {
  EnergySource,
  PowertrainType,
  SourceKind,
  Tariff,
  UnavailableReason,
  Vehicle,
  Zone,
} from './generated/types.gen';

/** The fleet as the service saw it at one instant, which is what freshness is measured against. */
export type FleetSnapshot = { serverTime: string; vehicles: Vehicle[] };

// The catalog, the service zone and the tariff are public, so nothing about the reader is sent
// with a request for them; a person who is signed in reads exactly what a visitor reads.
const anonymousRequest = { credentials: 'omit', cache: 'no-store', throwOnError: true } as const;

export async function fetchFleet(signal: AbortSignal): Promise<FleetSnapshot> {
  const { data } = await getVehicles({ ...anonymousRequest, signal });
  return { serverTime: data.server_time, vehicles: data.items };
}

export async function fetchServiceZones(signal: AbortSignal): Promise<Zone[]> {
  const { data } = await getZones({ ...anonymousRequest, signal });
  return data.items;
}

export async function fetchTariffs(signal: AbortSignal): Promise<Tariff[]> {
  const { data } = await getTariffs({ ...anonymousRequest, signal });
  return data.items;
}
