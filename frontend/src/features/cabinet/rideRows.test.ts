import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { RideSummary } from '../../shared/api/history.ts';
import { REASON_UNKNOWN, rideRow } from './rideRows.ts';

function ride(overrides: Partial<RideSummary> = {}): RideSummary {
  return {
    id: 'ride-1',
    vehicle: { id: 'vehicle-1', model: 'Demo Electric', powertrain_type: 'electric' },
    started_at: '2026-09-12T07:15:30.123456Z',
    completed_at: '2026-09-12T07:45:30.123456Z',
    invoice_id: 'invoice-1',
    completion: { reason: 'user_finished' },
    ...overrides,
  };
}

describe('one row of the ride feed', () => {
  test('names the vehicle, the moments and the reason', () => {
    const row = rideRow(ride());

    assert.equal(row.vehicle, 'Demo Electric');
    assert.equal(row.startedAt, '12 сентября 2026 г. в 13:15');
    assert.equal(row.completedAt, '12 сентября 2026 г. в 13:45');
    assert.equal(row.reason, 'Вручную');
  });

  test('states the reason of a ride the vehicle ended in one word', () => {
    const row = rideRow(ride({ completion: { reason: 'energy_depleted', exhausted_sources: ['battery'] } }));

    assert.equal(row.reason, 'Исчерпание');
  });

  test('names the invoice the ride was charged by, which the row links to', () => {
    assert.equal(rideRow(ride()).invoiceId, 'invoice-1');
  });

  // A reason this build does not know must not be written as one it does: a ride shown as ended by
  // hand when the service said something else would be the interface inventing the record.
  test('a reason this build does not know is written as unknown', () => {
    const unknown = { reason: 'towed' } as unknown as RideSummary['completion'];

    assert.equal(rideRow(ride({ completion: unknown })).reason, REASON_UNKNOWN);
  });

  test('a moment that cannot be read is written as missing rather than as a date', () => {
    const row = rideRow(ride({ started_at: 'not a moment' }));

    assert.equal(row.startedAt, '—');
  });
});
