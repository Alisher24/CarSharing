import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { FinishResult, InvoiceView } from '../../shared/api/current.ts';
import {
  clearCompletedRide,
  completedRide,
  COMPLETED_RIDE_MILLISECONDS,
  storeCompletedRide,
  storePayment,
  type CompletionStorage,
} from './completedRide.ts';

const OWNER = '01994342-6ba7-7000-8000-00000000000a';
const OTHER_OWNER = '01994342-6ba7-7000-8000-00000000000b';

/** The identifier of the invoice the fixture below was issued for. */
const INVOICE_ID = '01994342-6ba7-7000-8000-000000000003';

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

/**
 * The answer a finish gives, of which the summary shows the vehicle, the reason and the total. The
 * fields a stored record is judged by are the ones a real answer carries, so a fixture that left one
 * out would be refused by the reader rather than read.
 */
function finished(total: string, reason = 'user_finished'): FinishResult {
  return {
    server_time: '2026-09-14T07:30:30.123456Z',
    rental: {
      id: '01994342-6ba7-7000-8000-000000000001',
      state: 'completed',
      vehicle: { id: '01994342-6ba7-7000-8000-000000000002', model: 'Demo Electric' },
      completion: { reason },
      completed_at: '2026-09-14T07:30:30.123456Z',
      invoice_id: INVOICE_ID,
    },
    invoice: {
      version: '1',
      payment: { status: 'pending', updated_at: '2026-09-14T07:30:30.123456Z' },
      invoice: { id: INVOICE_ID, total_amount_tyiyn: total },
    },
  } as unknown as FinishResult;
}

/** One view of an invoice in the state a payment reached, which a payment answers with. */
function view(payment: InvoiceView['payment'], version: string): InvoiceView {
  return {
    version,
    payment,
    invoice: { id: INVOICE_ID, total_amount_tyiyn: '2789' },
  } as unknown as InvoiceView;
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

  test('carries the payment the ending published, from the answer itself', () => {
    const storage = storageOf();
    storeCompletedRide(OWNER, finished('2789'), 1_000, storage);

    const held = completedRide(OWNER, 2_000, storage);
    assert.equal(held?.payment.payment.status, 'pending');
    assert.equal(held?.payment.version, '1');
    assert.equal(held?.payment.invoice.id, INVOICE_ID);
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

describe('the payment kept beside a ride that was ended', () => {
  test('replaces the state of the invoice in the record the ending wrote', () => {
    const storage = storageOf();
    storeCompletedRide(OWNER, finished('2789'), 1_000, storage);

    storePayment(
      OWNER,
      INVOICE_ID,
      view({ status: 'paid', paid_at: '2026-09-14T07:31:00.000000Z' }, '2'),
      3_000,
      storage,
    );

    const held = completedRide(OWNER, 4_000, storage);
    assert.equal(held?.payment.payment.status, 'paid');
    assert.equal(held?.payment.version, '2');
    assert.equal(held?.receivedAt, 3_000, 'the payment did not move the moment the window is measured from');
    assert.equal(held?.finished.invoice.invoice.total_amount_tyiyn, '2789', 'the ending itself was rewritten');
  });

  test('is refused for another account and for another invoice', () => {
    const storage = storageOf();
    storeCompletedRide(OWNER, finished('2789'), 1_000, storage);

    storePayment(
      OTHER_OWNER,
      INVOICE_ID,
      view({ status: 'paid', paid_at: '2026-09-14T07:31:00.000000Z' }, '2'),
      3_000,
      storage,
    );
    storePayment(
      OWNER,
      '01994342-6ba7-7000-8000-0000000000ff',
      view({ status: 'paid', paid_at: 'x' }, '9'),
      3_000,
      storage,
    );

    const held = completedRide(OWNER, 4_000, storage);
    assert.equal(held?.payment.payment.status, 'pending');
    assert.equal(held?.payment.version, '1');
  });

  test('is not written when there is no ending to write it beside', () => {
    const storage = storageOf();
    storePayment(OWNER, INVOICE_ID, view({ status: 'paid', paid_at: 'x' }, '2'), 3_000, storage);

    assert.equal(completedRide(OWNER, 4_000, storage), undefined);
  });

  test('falls back to the view the ending carried when a record states no payment of its own', () => {
    const carried = finished('2789');
    const record = JSON.stringify({ owner: OWNER, receivedAt: 1_000, finished: carried });

    const held = completedRide(OWNER, 2_000, storageOf(record));
    assert.equal(held?.payment.payment.status, 'pending');
    assert.equal(held?.payment.version, '1');
  });
});
