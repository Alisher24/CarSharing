import type { Vehicle } from '../api/catalog.ts';

/** A vehicle stated by everything a check of the fleet reads: its identity, its version and its data. */
export function vehicle(id: string, version: string, model = `Модель ${id}`): Vehicle {
  return {
    id,
    version,
    model,
    status: 'available',
    powertrain_type: 'electric',
    position: { type: 'Point', coordinates: [74.6, 42.87] },
    energy_sources: [],
    telemetry_at: '2026-09-12T07:15:30.123456Z',
    telemetry_status: 'fresh',
  };
}
