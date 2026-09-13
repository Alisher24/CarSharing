import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import {
  cancelledSnapshot,
  collection,
  EXPIRES_AT,
  expiredSnapshot,
  MODEL,
  RENTAL,
  reservedSnapshot,
  ridingSnapshot,
  SERVER_TIME,
  WARNING,
  warningNotification,
} from './fixtures.ts';
import { currentWarning } from './currentWarning.ts';

describe('the warning a person is shown', () => {
  test('names the vehicle of the reservation in force and the moment it ends', () => {
    const warning = currentWarning(collection(SERVER_TIME, [warningNotification(WARNING, RENTAL)]), reservedSnapshot());

    assert.deepEqual(warning, {
      notificationId: WARNING,
      vehicle: MODEL,
      expiresAt: EXPIRES_AT,
      serverTime: SERVER_TIME,
    });
  });

  test('is not shown once the server has deactivated the warning', () => {
    const stopped = collection(SERVER_TIME, [warningNotification(WARNING, RENTAL, { active: false, version: '2' })]);

    assert.equal(currentWarning(stopped, reservedSnapshot()), undefined);
  });

  // Read state belongs to the server: a warning that was read is not a new warning after a reload.
  test('is not shown once it has been read', () => {
    const read = collection(SERVER_TIME, [
      warningNotification(WARNING, RENTAL, { version: '2', readAt: '2026-09-13T07:14:45.000000Z' }),
    ]);

    assert.equal(currentWarning(read, reservedSnapshot()), undefined);
  });

  test('is not shown when the reservation it names is no longer the one in force', () => {
    const other = reservedSnapshot('01994342-6ba7-7000-8000-000300000002');

    assert.equal(currentWarning(collection(SERVER_TIME, [warningNotification(WARNING, RENTAL)]), other), undefined);
  });

  // A reservation that was given back, that ran out or that became a ride is not one to warn about:
  // the server deactivates its warning, and the client does not wait to be told.
  test('is not shown for a reservation that has left the reservation', () => {
    const warning = collection(SERVER_TIME, [warningNotification(WARNING, RENTAL)]);

    for (const snapshot of [cancelledSnapshot(), expiredSnapshot(), ridingSnapshot()]) {
      assert.equal(currentWarning(warning, snapshot), undefined, 'a rental that is not reserved was warned about');
    }
  });

  test('is not shown when there is no reservation, no collection or no warning in it', () => {
    const withoutReservation = {
      kind: 'none',
      daily_limit: { available: true, resets_at: EXPIRES_AT },
      server_time: SERVER_TIME,
    } as const;
    const warning = collection(SERVER_TIME, [warningNotification(WARNING, RENTAL)]);

    assert.equal(currentWarning(undefined, reservedSnapshot()), undefined);
    assert.equal(currentWarning(warning, undefined), undefined);
    assert.equal(currentWarning(warning, withoutReservation), undefined);
    assert.equal(currentWarning(collection(SERVER_TIME, []), reservedSnapshot()), undefined);
  });

  // The contract declares a second kind of notification; only the warning about a reservation is one.
  test('is not a notification of another kind', () => {
    const completed = collection(SERVER_TIME, [
      {
        id: WARNING,
        type: 'rental_completed',
        rental_id: RENTAL,
        invoice_id: '01994342-6ba7-7000-8000-000500000001',
        completion: { reason: 'user_finished' },
        created_at: SERVER_TIME,
        version: '1',
        active: true,
      },
    ]);

    assert.equal(currentWarning(completed, reservedSnapshot()), undefined);
  });
});
