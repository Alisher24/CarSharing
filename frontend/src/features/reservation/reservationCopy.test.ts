import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { CurrentSnapshot } from '../../shared/api/current.ts';
import type { Countdown } from './countdown.ts';
import {
  currentRental,
  limitAllowsBooking,
  limitText,
  rateTextOf,
  refusalText,
  sameRates,
  warningText,
  BOOK_ACTION,
} from './reservationCopy.ts';

/** One answer of the private read, with the allowance it publishes. */
function snapshot(available: boolean, resetsAt = '2026-09-13T18:00:00.000000Z'): CurrentSnapshot {
  return {
    kind: 'none',
    server_time: '2026-09-13T07:15:30.000000Z',
    daily_limit: { available, resets_at: resetsAt },
  };
}

describe('the day of free reservations as it is shown', () => {
  test('offers the booking while the allowance is there', () => {
    assert.equal(limitAllowsBooking(snapshot(true)), true);
    assert.equal(limitText(snapshot(true)), BOOK_ACTION);
  });

  test('states the spent allowance with the moment it returns, in the service timezone', () => {
    // The moment arrives in UTC and is read where the service keeps its day: 18:00 UTC is midnight
    // in Bishkek, which is the day the next free reservation belongs to.
    const text = limitText(snapshot(false));
    assert.match(text, /Бесплатная бронь использована\. Следующая доступна/);
    assert.match(text, /14 сентября/);
    assert.match(text, /00:00/);
  });

  test('never presents an unreadable allowance as permission to book', () => {
    assert.equal(limitAllowsBooking(undefined), false);
    assert.equal(limitAllowsBooking(snapshot(false)), false);
    assert.match(limitText(undefined), /неизвестно/);
  });
});

describe('the rental a current answer holds', () => {
  test('is the one the answer names, and nothing when the answer names none', () => {
    assert.equal(currentRental(snapshot(true)), undefined);
    assert.equal(currentRental(undefined), undefined);
  });
});

describe('the conditions a reservation was made under', () => {
  test('are written as prices, and a rate that cannot be read is left out', () => {
    const rates = rateTextOf({
      id: '01994342-6ba7-7000-8000-000300000001',
      currency: 'KGS',
      billing_policy: 'per_mode_started_minute_v1',
      driving_rate_tyiyn_per_started_minute: '1234',
      paused_rate_tyiyn_per_started_minute: '321',
      version: '1',
    });

    assert.equal(rates.driving, '12,34 сома');
    assert.equal(rates.paused, '3,21 сома');
    assert.equal(sameRates(rates, { driving: '12,34 сома', paused: '3,21 сома' }), true);
    assert.equal(sameRates(rates, { driving: '15,00 сома', paused: '3,21 сома' }), false);
  });
});

describe('the wording of a refusal', () => {
  test('is chosen by the contract code', () => {
    assert.match(refusalText('DAILY_LIMIT_REACHED'), /использована/);
    assert.match(refusalText('VEHICLE_UNAVAILABLE'), /недоступен/);
  });

  test('names a refusal the table does not know as an unexplained one', () => {
    assert.match(refusalText('INTERNAL_ERROR'), /не выполнил команду/);
  });
});

describe('the warning that a reservation is running out', () => {
  const expiring = { vehicle: 'Демо Бензин 3', expiresAt: '2026-09-13T07:15:00.000000Z' };

  test('names the vehicle, the moment the reservation ends and what is left', () => {
    const left: Countdown = { state: 'left', milliseconds: 45_000, text: '0:45' };
    const text = warningText(expiring, left);

    assert.equal(text?.vehicle, 'Демо Бензин 3');
    assert.equal(text?.deadline, 'Бронь закончится 13 сентября в 13:15');
    assert.equal(text?.remaining, 'Осталось 0:45');
  });

  // The last minute is over at the deadline, and a moment that cannot be read is not one to warn
  // about: in both cases there is nothing left to say about the reservation.
  test('says nothing once the deadline has been reached or nothing can be counted down', () => {
    assert.equal(warningText(expiring, { state: 'due' }), undefined);
    assert.equal(warningText(expiring, { state: 'unreadable' }), undefined);
    assert.equal(warningText(expiring, undefined), undefined);
    assert.equal(
      warningText(
        { vehicle: 'Демо Бензин 3', expiresAt: 'not a moment' },
        { state: 'left', milliseconds: 1, text: '0:01' },
      ),
      undefined,
    );
  });
});
