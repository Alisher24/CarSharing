import type { LiveRental, Progress, Rental, TariffSnapshot, Vehicle } from '../../shared/api/current.ts';
import type { StartedRental } from './ridePace.ts';

/**
 * One rental as the contract states it, in whichever state a check is about. The fields a state does
 * not publish are absent rather than empty, so a check that reads the progress of a reservation would
 * be reading a value the contract never sends.
 */

/** The moment the answer was computed at. */
export const SERVER_TIME = '2026-09-13T07:15:30.000000Z';

/** The local moment that answer arrived, one second after the moment it states. */
export const RECEIVED_AT = new Date('2026-09-13T07:15:31.000Z');

/** A clock as a view builds it from one answer: the moment it states and when it arrived here. */
export const CLOCK = { serverTime: SERVER_TIME, receivedAt: RECEIVED_AT };

/** A moment that many seconds after the answer arrived, which is where the local clock has moved to. */
export function clockAtOffset(seconds: number): Date {
  return new Date(RECEIVED_AT.getTime() + seconds * 1_000);
}

/** What a ride in the middle of its first interval publishes: a minute and a half, and one tyiyn. */
export const PROGRESS: Progress = {
  driving_duration_microseconds: '90000000',
  driving_started_minutes: '2',
  estimated_amount_tyiyn: '1',
  paused_duration_microseconds: '1000000',
  paused_started_minutes: '1',
};

const ID = '01994342-6ba7-7000-8000-000200000009';

function vehicle(): Vehicle {
  return {
    id: '01994342-6ba7-7000-8000-000100000001',
    version: '1',
    model: 'Демо Бензин 3',
    status: 'available',
    powertrain_type: 'gasoline',
    position: { type: 'Point', coordinates: [74.6, 42.87] },
    energy_sources: [],
    telemetry_at: '2026-09-12T07:15:30.123456Z',
    telemetry_status: 'fresh',
  };
}

function tariffSnapshot(): TariffSnapshot {
  return {
    id: '01994342-6ba7-7000-8000-000200000001',
    currency: 'KGS',
    billing_policy: 'per_mode_started_minute_v1',
    driving_rate_tyiyn_per_started_minute: '1500',
    paused_rate_tyiyn_per_started_minute: '500',
    version: '1',
  };
}

/** One reservation, waiting for the ride to begin. */
export function reservation(): LiveRental {
  return {
    id: ID,
    state: 'reserved',
    reserved_at: '2026-09-13T07:00:00.000000Z',
    expires_at: '2026-09-13T07:15:30.000000Z',
    tariff_snapshot: tariffSnapshot(),
    vehicle: vehicle(),
    version: '2',
  };
}

/** One ride in the state it is asked for, with the progress the service publishes for it. */
export function startedRide(state: StartedRental['state']): StartedRental {
  return {
    id: ID,
    state,
    reserved_at: '2026-09-13T07:00:00.000000Z',
    started_at: SERVER_TIME,
    mode_started_at: SERVER_TIME,
    progress: PROGRESS,
    tariff_snapshot: tariffSnapshot(),
    vehicle: vehicle(),
    version: '3',
  };
}

/** One rental that is over, released by the deadline of its reservation. */
export function expiredRental(): Rental {
  return {
    id: ID,
    state: 'expired',
    reserved_at: '2026-09-13T07:00:00.000000Z',
    expired_at: SERVER_TIME,
    tariff_snapshot: tariffSnapshot(),
    vehicle: vehicle(),
    version: '3',
  };
}
