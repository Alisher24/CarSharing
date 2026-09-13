import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { countdownAt, remainingText, type Deadline } from './countdown.ts';

/** When the answer arrived here, which is what the countdown is anchored to. */
const receivedAt = new Date('2026-09-13T07:15:31.000Z');

function deadline(expiresAt: string, serverTime = '2026-09-13T07:15:30.000000Z'): Deadline {
  return { expiresAt, serverTime, receivedAt };
}

describe('the time left of a reservation', () => {
  test('is measured between the stored moments rather than from the moment of reading', () => {
    const held = deadline('2026-09-13T07:30:30.000000Z');

    assert.deepEqual(countdownAt(held, new Date('2026-09-13T07:15:31.000Z')), {
      state: 'left',
      milliseconds: 900_000,
      text: '15:00',
    });
  });

  test('falls with the clock and reaches nothing at the deadline itself', () => {
    const held = deadline('2026-09-13T07:30:30.000000Z');

    const later = countdownAt(held, new Date('2026-09-13T07:20:30.000Z'));
    assert.equal(later.state, 'left');
    assert.equal(later.state === 'left' ? later.milliseconds : 0, 601_000);

    assert.deepEqual(countdownAt(held, new Date('2026-09-13T07:30:30.000Z')), {
      state: 'left',
      milliseconds: 1_000,
      text: '0:01',
    });

    // The answer arrived a second after the moment it states, so the deadline this browser counts
    // down to is that same second later, and it is due one second after the stored deadline.
    assert.deepEqual(countdownAt(held, new Date('2026-09-13T07:30:31.000Z')), { state: 'due' });
  });

  test('is corrected by a later answer rather than by the ticks that passed', () => {
    // A tab that was suspended for an hour: the local clock moved, the answer did not. The time
    // left follows the two stored moments and the moment the answer arrived.
    const held = deadline('2026-09-13T07:30:30.000000Z');
    assert.deepEqual(countdownAt(held, new Date('2026-09-13T08:15:31.000Z')), { state: 'due' });

    // Reconciliation brings a fresh answer, computed by the server, and the countdown recovers.
    const reconciled: Deadline = {
      expiresAt: '2026-09-13T08:30:30.000000Z',
      serverTime: '2026-09-13T08:15:30.000000Z',
      receivedAt: new Date('2026-09-13T08:15:31.000Z'),
    };
    assert.deepEqual(countdownAt(reconciled, new Date('2026-09-13T08:15:31.000Z')), {
      state: 'left',
      milliseconds: 900_000,
      text: '15:00',
    });
  });

  test('reads a moment it cannot parse as unreadable rather than as due', () => {
    assert.deepEqual(countdownAt(deadline('not a moment'), receivedAt), { state: 'unreadable' });
  });
});

describe('the text of a remaining time', () => {
  test('is minutes and seconds, rounded up so the last second is shown', () => {
    assert.equal(remainingText(900_000), '15:00');
    assert.equal(remainingText(1), '0:01');
    assert.equal(remainingText(59_001), '1:00');
    assert.equal(remainingText(61_000), '1:01');
  });
});
