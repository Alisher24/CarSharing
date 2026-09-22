import type { ErrorCode } from './api/generated/types.gen.ts';
import { SIGN_IN_TO_BOOK } from './copy.ts';

/**
 * Russian wording for each refusal the service can answer with, chosen by contract code so that the
 * text a person reads never depends on a message written for a developer.
 *
 * The table is keyed by the generated code union, so a code the contract publishes cannot be answered
 * by a word this module does not have: the key is checked against `ErrorCode`, and a code missing from
 * it is the fallback below, which is one sentence for everything unnamed rather than a silent blank.
 *
 * One table serves every operation, because the same code is met in more than one place: the entry
 * window, the reservation, the ride and the payment all read their wording from here.
 */
const REFUSAL_TEXT: Partial<Record<ErrorCode, string>> = {
  EMAIL_ALREADY_REGISTERED: 'Этот адрес уже зарегистрирован. Войдите в существующий аккаунт.',
  INVALID_CREDENTIALS: 'Неверный адрес или пароль.',
  VALIDATION_FAILED: 'Проверьте адрес электронной почты и пароль.',

  DAILY_LIMIT_REACHED: 'Бесплатная бронь на сегодня уже использована',
  VEHICLE_UNAVAILABLE: 'Автомобиль больше недоступен',
  ACTIVE_RENTAL_EXISTS: 'У вас уже есть действующая аренда',
  RESERVATION_EXPIRED: 'Срок брони истёк',
  RENTAL_COMPLETED: 'Аренда уже завершена',
  INVALID_RENTAL_STATE: 'Бронь в состоянии, которое не позволяет это действие',
  RESOURCE_NOT_FOUND: 'Бронь не найдена',

  IDEMPOTENCY_CONFLICT: 'Этот ключ команды уже использован для другой команды',
  IDEMPOTENCY_IN_PROGRESS: 'Такая же команда ещё выполняется, повторите через секунду',
  CSRF_INVALID: 'Сессия устарела. Войдите заново',

  // The one thing a person is told when the server refuses an operation for want of a session, and
  // the same sentence the control that offers the operation carries: one fact, one wording.
  AUTHENTICATION_REQUIRED: SIGN_IN_TO_BOOK,

  OUTSTANDING_INVOICE: 'Есть неоплаченный счёт: новая бронь недоступна',
  PAYMENT_IN_PROGRESS: 'Оплата уже выполняется, повторите попытку',
  OUTSIDE_SERVICE_ZONE: 'Автомобиль вне зоны обслуживания: вернитесь в зону и завершите поездку',
  TELEMETRY_STALE: 'Положение автомобиля давно не подтверждалось: подождите и повторите',

  ORIGIN_NOT_ALLOWED: 'Запрос отклонён. Откройте приложение по обычному адресу.',
  SERVICE_UNAVAILABLE: 'Сервис временно недоступен. Повторите попытку позже.',
};

/**
 * What a refusal this table does not name is explained with. It is not a silent failure: a person
 * reads that the service did not carry the operation out and that another attempt may help.
 *
 * A code reaches it in two cases. A malformed request or an internal fault is one this interface
 * cannot produce — the generated client writes the request — and inventing a sentence for it would be
 * claiming to know what happened. A code the contract adds and no operation of the interface can
 * answer yet is one whose wording is a decision about the interface rather than a gap here; it reads
 * as an unexplained refusal until that decision is made.
 */
export const UNEXPLAINED_REFUSAL = 'Сервис не выполнил запрос. Повторите попытку позже.';

/** The sentence one refusal code is shown with. */
export function refusalText(code: ErrorCode): string {
  return REFUSAL_TEXT[code] ?? UNEXPLAINED_REFUSAL;
}
