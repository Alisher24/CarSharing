import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { FinishResult } from '../../shared/api/current.ts';
import {
  clearCompletedRide,
  completedRide,
  COMPLETED_RIDE_MILLISECONDS,
  storeCompletedRide,
  type CompletionStorage,
} from './completedRide.ts';

const OWNER = '01994342-6ba7-7000-8000-00000000000a';
const OTHER_OWNER = '01994342-6ba7-7000-8000-00000000000b';

/** A storage of its own, so nothing a test writes reaches the browser or another test. */
function storageOf(held: string | null = null): CompletionStorage {
  const entries = new Map<string, string>();
  if (held !== null) entries.set('carsharing.completed-ride', held);

  return {
    getItem: (key) => entries.get(key) ?? null,
    setItem: (key, value) => void entries.set(key, value),
    removeItem: (key) => void entries.delete(key),
  };
}

/** The answer a finish gives, of which the summary shows the vehicle, the reason and the total. */
function finished(total: string, reason = 'user_finished'): FinishResult {
  return {
    server_time: '2026-09-14T07:30:30.123456Z',
    rental: {
      id: '01994342-6ba7-7000-8000-000000000001',
      state: 'completed',
      vehicle: { id: '01994342-6ba7-7000-8000-000000000002', model: 'Demo Electric' },
      completion: { reason },
      completed_at: '2026-09-14T07:30:30.123456Z',
      invoice_id: '01994342-6ba7-7000-8000-000000000003',
    },
    invoice: {
      version: '1',
      payment: { status: 'pending', updated_at: '2026-09-14T07:30:30.123456Z' },
      invoice: {
        id: '01994342-6ba7-7000-8000-000000000003',
        total_amount_tyiyn: total,
      },
    },
  } as unknown as FinishResult;
}

describe('the record of a ride that was ended', () => {
  test('is kept and read back for the account that ended it', () => {
    const storage = storageOf();
    storeCompletedRide(OWNER, finished('2789'), 1_000, storage);

    const held = completedRide(OWNER, 2_000, storage);
    assert.equal(held?.owner, OWNER);
    assert.equal(held?.receivedAt, 1_000);
    assert.equal(held?.finished.invoice.invoice.total_amount_tyiyn, '2789');
  });

  test('is not shown to another account', () => {
    const storage = storageOf();
    storeCompletedRide(OWNER, finished('2789'), 1_000, storage);

    assert.equal(completedRide(OTHER_OWNER, 2_000, storage), undefined);
    assert.equal(completedRide(undefined, 2_000, storage), undefined);
  });

  test('is forgotten once the account starts something new', () => {
    const storage = storageOf();
    storeCompletedRide(OWNER, finished('2789'), 1_000, storage);

    clearCompletedRide(storage);
    assert.equal(completedRide(OWNER, 2_000, storage), undefined);
  });

  test('is not shown past its window', () => {
    const storage = storageOf();
    storeCompletedRide(OWNER, finished('2789'), 1_000, storage);

    assert.notEqual(completedRide(OWNER, 1_000 + COMPLETED_RIDE_MILLISECONDS - 1, storage), undefined);
    assert.equal(completedRide(OWNER, 1_000 + COMPLETED_RIDE_MILLISECONDS, storage), undefined);
  });

  test('is refused when it cannot be read, rather than shown as blanks', () => {
    for (const unreadable of [
      'not json at all',
      '{"owner":"' + OWNER + '"}',
      '{"owner":"' + OWNER + '","receivedAt":1,"finished":{}}',
      '{"owner":"' + OWNER + '","receivedAt":"soon","finished":{"rental":{}}}',
      JSON.stringify({ owner: OWNER, receivedAt: 1, finished: { rental: { state: 'completed' } } }),
    ]) {
      assert.equal(completedRide(OWNER, 2_000, storageOf(unreadable)), undefined);
    }
  });

  test('refuses a storage that cannot be read or written without failing the command', () => {
    const refusing: CompletionStorage = {
      getItem: () => {
        throw new Error('storage is unavailable');
      },
      setItem: () => {
        throw new Error('storage is unavailable');
      },
      removeItem: () => {
        throw new Error('storage is unavailable');
      },
    };

    storeCompletedRide(OWNER, finished('2789'), 1_000, refusing);
    clearCompletedRide(refusing);
    assert.equal(completedRide(OWNER, 2_000, refusing), undefined);
  });
});
