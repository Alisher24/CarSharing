import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { Payment } from '../api/current.ts';
import {
  PAID_AT,
  PAYMENT_FAILED,
  PAYMENT_PAID,
  PAYMENT_PENDING,
  PAYMENT_STATE,
  PAYMENT_UNKNOWN,
  PAY_ACTION,
  RETRY_PAYMENT_ACTION,
  paidAtText,
  paymentActionText,
  paymentActionable,
  paymentText,
} from './paymentCopy.ts';
import { UNREADABLE_VALUE } from './fares.ts';

/** One state of a payment, as the contract publishes it. */
function payment(status: string): Payment {
  switch (status) {
    case 'pending':
      return { status: 'pending', updated_at: '2026-09-14T07:30:30.123456Z' };
    case 'failed':
      return { status: 'failed', failed_at: '2026-09-14T07:30:31.123456Z', failure_code: 'declined' };
    case 'paid':
      return { status: 'paid', paid_at: '2026-09-14T07:31:00.123456Z' };
    default:
      return { status } as unknown as Payment;
  }
}

describe('what each state of a payment is called', () => {
  test('names the three states the contract publishes', () => {
    assert.equal(paymentText(payment('pending')), PAYMENT_PENDING);
    assert.equal(paymentText(payment('failed')), PAYMENT_FAILED);
    assert.equal(paymentText(payment('paid')), PAYMENT_PAID);
  });

  test('names a state it does not know as unknown rather than as one it does', () => {
    assert.equal(paymentText(payment('refunded')), PAYMENT_UNKNOWN);
  });

  test('offers a control exactly where a payment can still change something', () => {
    assert.equal(paymentActionable(payment('pending')), true);
    assert.equal(paymentActionable(payment('failed')), true);
    assert.equal(paymentActionable(payment('paid')), false);
    assert.equal(paymentActionable(payment('refunded')), false);
  });

  test('offers the control each state asks for, and none for a settled invoice', () => {
    assert.equal(paymentActionText(payment('pending')), PAY_ACTION);
    assert.equal(paymentActionText(payment('failed')), RETRY_PAYMENT_ACTION);
    assert.equal(paymentActionText(payment('paid')), undefined);
    assert.equal(paymentActionText(payment('refunded')), undefined);
  });
});

describe('the moment a settled invoice was paid', () => {
  test('is written in the time zone the service states its days in', () => {
    // The moment of the fixture is 07:31 UTC, which is 13:31 in the zone the service states its days
    // in: a moment written in the browser's own zone would be a different hour.
    assert.equal(paidAtText(payment('paid')), '14 сентября в 13:31');
  });

  test('is missing for a state that carries no moment of settlement', () => {
    assert.equal(paidAtText(payment('pending')), undefined);
    assert.equal(paidAtText(payment('failed')), undefined);
    assert.equal(paidAtText(payment('refunded')), undefined);
  });

  test('is missing, rather than guessed at, when the moment cannot be read', () => {
    assert.equal(paidAtText({ status: 'paid', paid_at: 'yesterday' }), UNREADABLE_VALUE);
  });
});

describe('the wording of the payment', () => {
  test('names the state and the moment it is shown with', () => {
    assert.equal(PAYMENT_STATE, 'Оплата');
    assert.equal(PAID_AT, 'Оплачено');
    assert.equal(PAY_ACTION, 'Оплатить');
    assert.equal(RETRY_PAYMENT_ACTION, 'Повторить оплату');
  });
});
