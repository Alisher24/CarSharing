import type { CurrentSnapshot, Rental, TariffSnapshot } from '../../shared/api/current.ts';
import type {
  Notification,
  NotificationCollection,
  ReservationExpiringNotification,
} from '../../shared/api/notifications.ts';
import { vehicle } from '../events/fixtures.ts';

export const RENTAL = '01994342-6ba7-7000-8000-000300000001';
export const WARNING = '01994342-6ba7-7000-8000-000400000001';
export const SERVER_TIME = '2026-09-13T07:14:30.000000Z';
export const EXPIRES_AT = '2026-09-13T07:15:00.000000Z';
export const MODEL = 'Демо Бензин 3';

/** The warning one account holds about one reservation, as the collection publishes it. */
export function warningNotification(
  id: string,
  rentalId: string,
  { version = '1', active = true, readAt }: { version?: string; active?: boolean; readAt?: string } = {},
): ReservationExpiringNotification {
  return {
    id,
    type: 'reservation_expiring',
    rental_id: rentalId,
    expires_at: EXPIRES_AT,
    created_at: '2026-09-13T07:14:00.000000Z',
    version,
    active,
    read_at: readAt,
  };
}

/** One answer of the collection: the moment it was computed at and the notifications it published. */
export function collection(serverTime: string, items: readonly Notification[]): NotificationCollection {
  return { items: [...items], next_cursor: null, server_time: serverTime };
}

/**
 * One reservation in force, as the private read publishes it. Only the fields a warning reads are
 * stated, and the rental is the one the contract declares for the state it is in.
 */
export function reservedSnapshot(id = RENTAL, expiresAt = EXPIRES_AT): CurrentSnapshot {
  return {
    kind: 'rental',
    rental: {
      id,
      state: 'reserved',
      vehicle: vehicle('v1', '1', MODEL),
      reserved_at: '2026-09-13T07:00:00.000000Z',
      expires_at: expiresAt,
      tariff_snapshot: tariffSnapshot(),
      version: '1',
    },
    daily_limit: { available: false, resets_at: expiresAt },
    server_time: SERVER_TIME,
  };
}

/** One reservation that was given back, which is not the reservation in force any more. */
export function cancelledSnapshot(id = RENTAL): CurrentSnapshot {
  return snapshotOf({
    id,
    state: 'cancelled',
    vehicle: vehicle('v1', '1', MODEL),
    reserved_at: '2026-09-13T07:00:00.000000Z',
    cancelled_at: '2026-09-13T07:14:40.000000Z',
    tariff_snapshot: tariffSnapshot(),
    version: '2',
  });
}

/** One reservation whose deadline passed, which released it instead of warning about it. */
export function expiredSnapshot(id = RENTAL): CurrentSnapshot {
  return snapshotOf({
    id,
    state: 'expired',
    vehicle: vehicle('v1', '1', MODEL),
    reserved_at: '2026-09-13T07:00:00.000000Z',
    expired_at: EXPIRES_AT,
    tariff_snapshot: tariffSnapshot(),
    version: '2',
  });
}

/** One rental that has become a ride, which is no longer counted down to. */
export function ridingSnapshot(id = RENTAL): CurrentSnapshot {
  return snapshotOf({
    id,
    state: 'active',
    vehicle: vehicle('v1', '1', MODEL),
    reserved_at: '2026-09-13T07:00:00.000000Z',
    started_at: SERVER_TIME,
    mode_started_at: SERVER_TIME,
    progress: {
      driving_duration_microseconds: '0',
      driving_started_minutes: '0',
      estimated_amount_tyiyn: '0',
      paused_duration_microseconds: '0',
      paused_started_minutes: '0',
    },
    tariff_snapshot: tariffSnapshot(),
    version: '2',
  });
}

/** One snapshot of a reservation that has left `reserved`, which is what a warning is not about. */
function snapshotOf(rental: Rental): CurrentSnapshot {
  return { kind: 'rental', rental, daily_limit: { available: true, resets_at: EXPIRES_AT }, server_time: SERVER_TIME };
}

/** The rates one reservation was made under, which every rental of this fixture stores. */
function tariffSnapshot(): TariffSnapshot {
  return {
    id: '01994342-6ba7-7000-8000-000200000001',
    currency: 'KGS',
    billing_policy: 'per_mode_started_minute_v1',
    driving_rate_tyiyn_per_started_minute: '1234',
    paused_rate_tyiyn_per_started_minute: '321',
    version: '1',
  };
}
