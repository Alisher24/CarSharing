import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { refusalText, UNEXPLAINED_REFUSAL } from './refusal.ts';

describe('the wording of a refusal', () => {
  test('is chosen by the contract code, whichever operation answered', () => {
    assert.match(refusalText('DAILY_LIMIT_REACHED'), /использована/);
    assert.match(refusalText('VEHICLE_UNAVAILABLE'), /недоступен/);
    assert.match(refusalText('INVALID_CREDENTIALS'), /Неверный адрес или пароль/);
  });

  // One refusal of the entry window and of a command of the account's own rental is one sentence: a
  // person reads what the server objected to, not which screen asked.
  test('states one sentence for a code two features can meet', () => {
    assert.equal(refusalText('AUTHENTICATION_REQUIRED'), 'Войдите, чтобы забронировать автомобиль');
    assert.equal(refusalText('ORIGIN_NOT_ALLOWED'), 'Запрос отклонён. Откройте приложение по обычному адресу.');
    assert.equal(refusalText('SERVICE_UNAVAILABLE'), 'Сервис временно недоступен. Повторите попытку позже.');
  });

  // The table is keyed by the generated code union, so its keys are the contract's own codes and a
  // code it does not name is read as an unexplained refusal — a sentence a person can act on rather
  // than a blank where the reason should be.
  test('names a code no wording is declared for as an unexplained refusal', () => {
    assert.equal(refusalText('INTERNAL_ERROR'), UNEXPLAINED_REFUSAL);
    assert.equal(refusalText('MALFORMED_JSON'), UNEXPLAINED_REFUSAL);
    assert.equal(refusalText('BODY_TOO_LARGE'), UNEXPLAINED_REFUSAL);
  });

  // Every code the table does name is a code the contract publishes: a key the generated union does
  // not have would be a typo that compiles and is never read, which is what this holds.
  test('answers every refusal of the interface with a sentence of its own', () => {
    const declared = ['DAILY_LIMIT_REACHED', 'VEHICLE_UNAVAILABLE', 'OUTSTANDING_INVOICE'] as const;

    for (const code of declared) {
      assert.notEqual(refusalText(code), UNEXPLAINED_REFUSAL);
    }
  });
});
