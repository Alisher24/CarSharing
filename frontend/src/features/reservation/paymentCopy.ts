import type { Payment } from '../../shared/api/current.ts';
import { serviceMoment } from '../../shared/locale.ts';
import { UNREADABLE_VALUE } from './rideCopy.ts';

/**
 * The Russian wording of the payment of a finished ride, and the two rules that produce it: what each
 * state of a payment is called, and whether the state offers a control at all.
 *
 * The state written here is the one the service last published — the payment of the invoice the ride
 * was charged on, as that invoice was read — so the interface reads a status rather than deciding one.
 * Nothing here computes what is owed.
 */

/** What the invoice of a finished ride is waiting for, which is the attempt the service owes it. */
export const PAYMENT_PENDING = 'Ожидает оплаты';

/** What a refused attempt is reported as, with the control that asks for another one. */
export const PAYMENT_FAILED = 'Оплата отклонена';

/** What a settled invoice is reported as, with the moment it was settled. */
export const PAYMENT_PAID = 'Оплачено';

/** What is written before the state of the payment. */
export const PAYMENT_STATE = 'Оплата';

/** What is written before the moment a settled invoice was paid. */
export const PAID_AT = 'Оплачено';

/** What the control that pays an invoice offers while the service still owes its first attempt. */
export const PAY_ACTION = 'Оплатить';

/** What the control offers after an attempt that was refused. */
export const RETRY_PAYMENT_ACTION = 'Повторить оплату';

/** What a state this build does not know is written as, rather than as one it does. */
export const PAYMENT_UNKNOWN = 'Состояние оплаты неизвестно';

/** The sentence one state of a payment is shown with. */
export function paymentText(payment: Payment): string {
  switch (payment.status) {
    case 'pending':
      return PAYMENT_PENDING;
    case 'failed':
      return PAYMENT_FAILED;
    case 'paid':
      return PAYMENT_PAID;
    default:
      return PAYMENT_UNKNOWN;
  }
}

/**
 * Whether the state of a payment offers a control, which is what decides that the panel shows one. A
 * settled invoice has nothing left to ask for: a button that sent a payment for it would answer the
 * view that exists and change nothing.
 */
export function paymentActionable(payment: Payment): boolean {
  return payment.status === 'pending' || payment.status === 'failed';
}

/** What the control of one state offers, or undefined when the state offers none. */
export function paymentActionText(payment: Payment): string | undefined {
  switch (payment.status) {
    case 'pending':
      return PAY_ACTION;
    case 'failed':
      return RETRY_PAYMENT_ACTION;
    default:
      return undefined;
  }
}

/**
 * The moment a settled invoice was paid, in the timezone the service states its days in, or nothing
 * for a state that carries no moment of settlement. A moment the interface cannot read is written as
 * missing rather than guessed at.
 */
export function paidAtText(payment: Payment): string | undefined {
  if (payment.status !== 'paid') return undefined;

  return serviceMoment(payment.paid_at) ?? UNREADABLE_VALUE;
}
