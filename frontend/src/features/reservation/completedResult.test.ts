import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { Completion, FinishResult, InvoiceView, Payment } from '../../shared/api/current.ts';
import type {
  Notification,
  NotificationCollection,
  RentalCompletedNotification,
} from '../../shared/api/notifications.ts';
import type { CompletedRide } from './completedRide.ts';
import {
  completedNotification,
  completedResult,
  invoiceWorthReading,
  recordedResult,
  shownResult,
} from './completedResult.ts';

const OWNER = '01994342-6ba7-7000-8000-00000000000a';
const RENTAL = '01994342-6ba7-7000-8000-000000000001';
const OTHER_RENTAL = '01994342-6ba7-7000-8000-0000000000f1';
const INVOICE = '01994342-6ba7-7000-8000-000000000003';
const OTHER_INVOICE = '01994342-6ba7-7000-8000-0000000000f3';
const NOTICE = '01994342-6ba7-7000-8000-000000000004';
const OLDER_NOTICE = '01994342-6ba7-7000-8000-000000000005';
const WARNING = '01994342-6ba7-7000-8000-000000000006';
const SERVER_TIME = '2026-09-14T07:15:00.000000Z';

/** The moment the ride of the fixtures ended, which is ten minutes before it was noticed. */
const ENDED_AT = '2026-09-14T07:00:00.123456Z';

/** The moment the notification about that ride was written, which is not when the ride ended. */
const CREATED_AT = '2026-09-14T07:10:00.123456Z';

const MODEL = 'Демо Электро 1';

/** One notification about a ride that ended, as the collection publishes it. */
function completionNotice(
  id: string,
  {
    rentalId = RENTAL,
    invoiceId = INVOICE,
    completion = { reason: 'user_finished' },
    endedAt = ENDED_AT,
  }: {
    rentalId?: string;
    invoiceId?: string;
    completion?: Completion;
    endedAt?: string;
  } = {},
): RentalCompletedNotification {
  return {
    id,
    type: 'rental_completed',
    rental_id: rentalId,
    invoice_id: invoiceId,
    completion,
    created_at: CREATED_AT,
    ended_at: endedAt,
    version: '1',
    active: true,
  };
}

/** One notification about a reservation that is running out, which reports no ride that ended. */
function warningNotice(id: string): Notification {
  return {
    id,
    type: 'reservation_expiring',
    rental_id: RENTAL,
    expires_at: CREATED_AT,
    created_at: CREATED_AT,
    version: '1',
    active: true,
  };
}

/** One answer of the collection: the moment it was computed at and the notifications it published. */
function collection(items: readonly Notification[], serverTime = SERVER_TIME): NotificationCollection {
  return { items: [...items], next_cursor: null, server_time: serverTime };
}

/** One answer of the invoice read, of which the result shows the total and the state of the payment. */
function invoiceView(id: string, total: string, payment: Payment): InvoiceView {
  return { version: '1', payment, invoice: { id, total_amount_tyiyn: total } } as unknown as InvoiceView;
}

function pending(): Payment {
  return { status: 'pending', updated_at: CREATED_AT };
}

function failed(): Payment {
  return { status: 'failed', failed_at: CREATED_AT, failure_code: 'declined' };
}

function paid(): Payment {
  return { status: 'paid', paid_at: '2026-09-14T07:11:00.123456Z' };
}

/** The answer one finish gave, of which the record of a ride this tab ended is made. */
function finished(rentalId = RENTAL, total = '2789'): FinishResult {
  return {
    server_time: CREATED_AT,
    rental: {
      id: rentalId,
      state: 'completed',
      vehicle: { id: '01994342-6ba7-7000-8000-000000000002', model: MODEL },
      completion: { reason: 'user_finished' },
      completed_at: ENDED_AT,
      invoice_id: INVOICE,
    },
    invoice: invoiceView(INVOICE, total, pending()),
  } as unknown as FinishResult;
}

/** What this tab wrote when it ended the ride of the fixtures, with the payment it last published. */
function recordOf(payment: Payment = pending(), rentalId = RENTAL, total = '2789'): CompletedRide {
  return {
    owner: OWNER,
    receivedAt: 1_000,
    finished: finished(rentalId, total),
    payment: invoiceView(INVOICE, total, payment),
  };
}

describe('the notification a completed ride is read from', () => {
  test('is the newest one about a ride that ended, whatever else the account was told', () => {
    const held = collection([warningNotice(WARNING), completionNotice(NOTICE), completionNotice(OLDER_NOTICE)]);

    assert.equal(completedNotification(held)?.id, NOTICE);
  });

  test('is nothing when the account holds no notification about a ride that ended', () => {
    assert.equal(completedNotification(collection([warningNotice(WARNING)])), undefined);
    assert.equal(completedNotification(collection([])), undefined);
    assert.equal(completedNotification(undefined), undefined);
  });
});

describe('the result of a completed ride', () => {
  test('states the reason, the invoice and the moment the ride ended rather than when it was noticed', () => {
    const result = completedResult(completionNotice(NOTICE), invoiceView(INVOICE, '2789', pending()));

    assert.deepEqual(result, {
      rentalId: RENTAL,
      completion: { reason: 'user_finished' },
      endedAt: ENDED_AT,
      charge: { invoiceId: INVOICE, totalAmountTyiyn: '2789', payment: pending() },
    });
  });

  test('carries the sources that ran out, which is what an ending without energy names', () => {
    const completion: Completion = { reason: 'energy_depleted', exhausted_sources: ['battery'] };
    const result = completedResult(completionNotice(NOTICE, { completion }), undefined);

    assert.deepEqual(result?.completion, completion);
  });

  test('states the reason and the moment before the invoice has been read', () => {
    const result = completedResult(completionNotice(NOTICE), undefined);

    assert.equal(result?.charge, undefined);
    assert.equal(result?.endedAt, ENDED_AT);
  });

  test('refuses the invoice of another ride, which is a read the notification does not name', () => {
    const stale = invoiceView(OTHER_INVOICE, '2789', paid());

    assert.equal(completedResult(completionNotice(NOTICE), stale)?.charge, undefined);
  });

  test('is nothing without a notification, however much of an invoice was read', () => {
    assert.equal(completedResult(undefined, invoiceView(INVOICE, '2789', paid())), undefined);
  });
});

describe('the result of a ride this tab ended itself', () => {
  test('names the vehicle and states the payment the service published beside the ending', () => {
    assert.deepEqual(recordedResult(recordOf(paid())), {
      rentalId: RENTAL,
      vehicle: MODEL,
      completion: { reason: 'user_finished' },
      endedAt: ENDED_AT,
      charge: { invoiceId: INVOICE, totalAmountTyiyn: '2789', payment: paid() },
    });
  });
});

describe('the result the panel shows', () => {
  const published = completedResult(completionNotice(NOTICE), invoiceView(INVOICE, '2789', paid()));

  test('is what the service published whenever there is any of it', () => {
    const shown = shownResult(published, recordOf(pending()));

    assert.equal(shown?.charge?.payment.status, 'paid', 'the record replaced the state the service published');
  });

  test('names the vehicle the record of the same ride holds, which no published answer carries', () => {
    assert.equal(shownResult(published, recordOf(pending()))?.vehicle, MODEL);
  });

  test('names no vehicle out of a record of another ride', () => {
    const shown = shownResult(published, recordOf(pending(), OTHER_RENTAL));

    assert.equal(shown?.vehicle, undefined);
    assert.equal(shown?.rentalId, RENTAL);
  });

  test('is the record while the notification about that ride has not been read yet', () => {
    const shown = shownResult(undefined, recordOf(pending()));

    assert.equal(shown?.vehicle, MODEL);
    assert.equal(shown?.charge?.payment.status, 'pending');
  });

  test('is nothing when the service published none and this tab ended none', () => {
    assert.equal(shownResult(undefined, undefined), undefined);
  });
});

describe('whether the invoice of a completed ride is worth reading again', () => {
  test('is worth reading until something has been read at all', () => {
    assert.equal(invoiceWorthReading(undefined), true);
  });

  // The service owes a pending invoice its first attempt, and makes it without the client asking.
  test('is worth reading while the payment is pending', () => {
    assert.equal(invoiceWorthReading(pending()), true);
  });

  test('is not worth reading once the payment has settled', () => {
    assert.equal(invoiceWorthReading(paid()), false);
    assert.equal(invoiceWorthReading(failed()), false);
  });
});
