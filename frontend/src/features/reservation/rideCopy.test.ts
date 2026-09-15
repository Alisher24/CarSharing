import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { Completion, Progress, SourceKind } from '../../shared/api/current.ts';
import { commandText, type CommandPhase } from './commandPhase.ts';
import {
  amountText,
  completionText,
  COMPLETION_UNKNOWN,
  FINISH_ACTION,
  FINISH_QUESTION,
  FINISH_WARNING,
  finishedAtText,
  invoiceAmountText,
  KEEP_RIDING_ACTION,
  PAUSE_ACTION,
  RESUME_ACTION,
  START_ACTION,
  UNREADABLE_VALUE,
} from './rideCopy.ts';

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
    assert.equal(FINISH_ACTION, 'Завершить поездку');
    assert.equal(KEEP_RIDING_ACTION, 'Продолжить поездку');
  });

  test('ask before a ride is ended, because an ending is not undone by asking again', () => {
    assert.match(FINISH_QUESTION, /Завершить поездку\?/);
    assert.match(FINISH_WARNING, /Счёт/);
  });
});

describe('what an ending is reported as', () => {
  test('is named while the ending is on its way', () => {
    assert.equal(commandText({ state: 'sending', action: 'finish' }), 'Завершаем поездку…');
  });

  test('explains the two refusals an ending can meet where the ride stands', () => {
    const outside = commandText({ state: 'refused', action: 'finish', code: 'OUTSIDE_SERVICE_ZONE' });
    const stale = commandText({ state: 'refused', action: 'finish', code: 'TELEMETRY_STALE' });

    assert.match(outside ?? '', /вне зоны/);
    assert.match(stale ?? '', /подтверждалось/);
    assert.notEqual(outside, stale);
  });
});

describe('what a completed ride is reported with', () => {
  test('writes the total of the invoice as som, exactly as the invoice states it', () => {
    assert.equal(invoiceAmountText('2789'), '27,89 сома');
    assert.equal(invoiceAmountText('900719925474099399').replaceAll('\u00a0', ' '), '9 007 199 254 740 993,99 сома');
    assert.equal(invoiceAmountText('не число'), UNREADABLE_VALUE);
  });

  test('writes the moment the ride ended in the zone the service states its days in', () => {
    // The moment of the fixture is 07:30 UTC, which is 13:30 in the zone the service states its days
    // in: a moment written in the browser's own zone would be a different hour.
    assert.equal(finishedAtText('2026-09-14T07:30:30.123456Z'), '14 сентября в 13:30');
  });

  test('writes a moment it cannot read as missing rather than as a guess', () => {
    assert.equal(finishedAtText('вчера'), UNREADABLE_VALUE);
  });
});

describe('what ended a ride', () => {
  /** One ending caused by the sources that ran out, which is what the reason is read with. */
  function exhaustion(exhausted: SourceKind[]): Completion {
    return { reason: 'energy_depleted', exhausted_sources: exhausted };
  }

  test('names every source that ran out, in the words the fleet names them by', () => {
    assert.equal(completionText(exhaustion(['battery'])), 'закончился запас энергии или топлива: батарея');
    assert.equal(completionText(exhaustion(['gasoline'])), 'закончился запас энергии или топлива: бензин');
    assert.equal(completionText(exhaustion(['diesel'])), 'закончился запас энергии или топлива: дизель');
    assert.equal(completionText(exhaustion(['lpg'])), 'закончился запас энергии или топлива: сжиженный газ');
    assert.equal(completionText(exhaustion(['cng'])), 'закончился запас энергии или топлива: сжатый газ');
  });

  test('lists the sources in the order the service reported them', () => {
    assert.equal(
      completionText(exhaustion(['gasoline', 'lpg'])),
      'закончился запас энергии или топлива: бензин, сжиженный газ',
    );
  });

  test('writes the reason alone when no source was named and when the person ended the ride', () => {
    assert.equal(completionText(exhaustion([])), 'закончился запас энергии или топлива');
    assert.equal(completionText({ reason: 'user_finished' }), 'поездку завершил пользователь');
  });

  test('names a source and a reason this build does not know rather than leaving a gap', () => {
    const unknown = completionText(exhaustion(['hydrogen' as unknown as SourceKind]));

    assert.equal(unknown, 'закончился запас энергии или топлива: неизвестный источник');
    assert.equal(completionText({ reason: 'towed' } as unknown as Completion), COMPLETION_UNKNOWN);
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
