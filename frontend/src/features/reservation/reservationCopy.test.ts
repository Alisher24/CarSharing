import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { CurrentSnapshot } from '../../shared/api/current.ts';
import {
  bishkekMoment,
  currentRental,
  limitAllowsBooking,
  limitText,
  rateTextOf,
  refusalText,
  sameRates,
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

  test('writes a moment it cannot read as nothing rather than as a date', () => {
    assert.equal(bishkekMoment('not a moment'), undefined);
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
