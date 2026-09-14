import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { Progress } from '../../shared/api/current.ts';
import { commandText, type CommandPhase } from './commandPhase.ts';
import { amountText, PAUSE_ACTION, RESUME_ACTION, START_ACTION } from './rideCopy.ts';

/** One progress statement, with the amount a test is about. */
function progress(estimatedAmountTyiyn: string): Progress {
  return {
    driving_duration_microseconds: '90000000',
    driving_started_minutes: '2',
    estimated_amount_tyiyn: estimatedAmountTyiyn,
    paused_duration_microseconds: '1000000',
    paused_started_minutes: '1',
  };
}

describe('what a ride costs so far', () => {
  test('is the estimate the service published, divided into som exactly', () => {
    assert.equal(amountText(progress('1234')), '12,34 сома');
    assert.equal(amountText(progress('0')), '0,00 сома');
  });

  test('keeps every tyiyn of an amount larger than a floating-point number can hold', () => {
    // An amount past two to the fifty-third tyiyn: read as a number it would lose its last digits.
    // The groups are separated by the mark the interface locale writes, which is a no-break space.
    const amount = amountText(progress('900719925474099399'));

    assert.equal(amount.replaceAll('\u00a0', ' '), '9 007 199 254 740 993,99 сома');
  });

  test('is written as missing rather than as zero when the amount cannot be read', () => {
    assert.equal(amountText(progress('не число')), '—');
    assert.equal(amountText(progress('-1')), '—');
  });
});

describe('the controls of a ride', () => {
  test('name the one command each state allows', () => {
    assert.equal(START_ACTION, 'Начать поездку');
    assert.equal(PAUSE_ACTION, 'Пауза');
    assert.equal(RESUME_ACTION, 'Продолжить');
  });
});

describe('what a command is reported as', () => {
  test('names each command while it is on its way, and nothing once it is settled', () => {
    assert.equal(commandText({ state: 'sending', action: 'start' }), 'Начинаем поездку…');
    assert.equal(commandText({ state: 'sending', action: 'pause' }), 'Ставим поездку на паузу…');
    assert.equal(commandText({ state: 'sending', action: 'resume' }), 'Продолжаем поездку…');
    assert.equal(commandText({ state: 'done', action: 'start', replayed: false }), undefined);
    assert.equal(commandText({ state: 'idle' }), undefined);
  });

  test('says that an outcome is unknown instead of pretending to know it', () => {
    assert.match(commandText({ state: 'unknown', action: 'pause' }) ?? '', /неизвестен/);
  });

  test('names what the server objected to, which is not what an unknown answer says', () => {
    const refused = commandText({ state: 'refused', action: 'start', code: 'RESERVATION_EXPIRED' });
    const unknown = commandText({ state: 'unknown', action: 'start' });

    assert.match(refused ?? '', /Срок брони истёк/);
    assert.notEqual(refused, unknown);
    assert.match(commandText({ state: 'refused', action: 'pause', code: 'RENTAL_COMPLETED' }) ?? '', /завершена/);
  });

  test('asks an account whose session ended to sign in rather than to repeat', () => {
    const signedOut: CommandPhase = { state: 'signed-out' };

    assert.match(commandText(signedOut) ?? '', /Войдите/);
  });
});
