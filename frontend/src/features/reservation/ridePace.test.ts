import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import {
  drivingDuration,
  durationText,
  elapsedInMode,
  hasStarted,
  pausedDuration,
  rideModeOf,
  rideModeText,
} from './ridePace.ts';
import { CLOCK, clockAtOffset, PROGRESS, RECEIVED_AT, reservation, SERVER_TIME, startedRide } from './rideFixture.ts';

describe('the mode a rental is in', () => {
  test('is read from the state the server published, not from what was asked for', () => {
    assert.equal(rideModeOf(startedRide('active')), 'driving');
    assert.equal(rideModeOf(startedRide('paused')), 'paused');
  });

  test('is named in Russian, one word per mode', () => {
    assert.equal(rideModeText('driving'), 'Движение');
    assert.equal(rideModeText('paused'), 'Пауза');
  });

  test('belongs to a ride that has started rather than to every rental', () => {
    assert.equal(hasStarted(startedRide('active')), true);
    assert.equal(hasStarted(startedRide('paused')), true);
    assert.equal(hasStarted(reservation()), false);
  });
});

describe('the time a ride has spent in its current mode', () => {
  const moment = { modeStartedAt: SERVER_TIME, ...CLOCK };

  test('is measured between the stored moment and the moment the server is at', () => {
    assert.equal(elapsedInMode(moment, RECEIVED_AT), 0n);
    assert.equal(elapsedInMode(moment, clockAtOffset(90)), 90_000_000n);
  });

  test('follows the server moment rather than a count of the ticks that passed', () => {
    // A tab that was closed for two hours: the local clock moved, the stored moments did not.
    const later = elapsedInMode(moment, clockAtOffset(7_200));

    assert.equal(later, 7_200_000_000n);
    assert.equal(durationText(String(later)), '120:00');
  });

  test('reads a moment it cannot parse as nothing rather than as no time at all', () => {
    assert.equal(elapsedInMode({ ...moment, modeStartedAt: 'not a moment' }, RECEIVED_AT), undefined);
    assert.equal(elapsedInMode({ ...moment, serverTime: 'not a moment' }, RECEIVED_AT), undefined);
  });

  test('never counts below zero a mode the answer states as starting after the server moment', () => {
    const ahead = { ...moment, modeStartedAt: '2026-09-13T07:15:40.000000Z' };

    assert.equal(elapsedInMode(ahead, RECEIVED_AT), 0n);
  });
});

describe('the progress a ride publishes', () => {
  test('states each mode duration as the exact digits the service sent', () => {
    assert.equal(drivingDuration(PROGRESS), '90000000');
    assert.equal(pausedDuration(PROGRESS), '1000000');
  });

  test('is written as minutes and seconds even past what a floating-point number can hold', () => {
    // A duration beyond two to the fifty-third microseconds is one no floating-point number can name
    // exactly, so a value that went through one would lose its last digits here. The microseconds
    // themselves are left out of a riding duration, which is why only the seconds are written.
    assert.equal(durationText('9007199254740995'), '150119987:34');
  });
});

describe('a duration as it is written', () => {
  test('is minutes and seconds, each two digits wide', () => {
    assert.equal(durationText('0'), '00:00');
    assert.equal(durationText('1000000'), '00:01');
    assert.equal(durationText('90000000'), '01:30');
    assert.equal(durationText('3600000000'), '60:00');
  });

  test('leaves the microseconds out rather than rounding the interval up', () => {
    assert.equal(durationText('1999999'), '00:01');
  });

  test('is nothing at all when the value is not a whole, non-negative number of microseconds', () => {
    assert.equal(durationText('не число'), undefined);
    assert.equal(durationText('-1000000'), undefined);
    assert.equal(durationText('1.5'), undefined);
  });
});
