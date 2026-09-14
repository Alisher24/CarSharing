import type { ApiError, CurrentSnapshot, Rental, TariffSnapshot } from '../../shared/api/current.ts';
import { SERVICE_TIME_ZONE } from '../../shared/locale.ts';
import { somText } from '../fleet/money.ts';
import type { Countdown } from './countdown.ts';

/**
 * The Russian wording of the reservation, and the two rules that produce it: how a moment of the
 * service day is written, and what the day's allowance is called. Both live here rather than in each
 * component that shows them, so the panel and the card cannot describe one state two ways.
 */

/** What the booking control says before anything was asked. */
export const BOOK_ACTION = 'Забронировать на 15 минут';

/** The heading of the step that asks a person to agree to the conditions before they are taken. */
export const CONFIRM_HEADING = 'Подтверждение брони';

/** What a person is warned about, in the words the specification fixes. */
export const FREE_RESERVATION_WARNING =
  'Одна бесплатная бронь в день. После отмены или истечения лимит не восстанавливается';

/** The control that confirms the booking, in the words the specification fixes. */
export const CONFIRM_ACTION = 'Использовать бесплатную бронь';

/** What the confirmation says the reservation costs and how long it stands. */
export const FREE_PERIOD = '15 минут бесплатно';

/** The heading of the panel that shows the reservation in force. */
export const PANEL_HEADING = 'Текущая бронь';

/** What the panel offers while a reservation is running. */
export const CANCEL_ACTION = 'Отменить бронь';

/** What cancelling a reservation asks, in the words the specification fixes. */
export const CANCEL_QUESTION = 'Отменить бронь? Автомобиль станет доступен другим пользователям';

/** The control that keeps a reservation a person asked about cancelling. */
export const KEEP_ACTION = 'Оставить бронь';

/** What the cancellation keeps warning about, so nobody cancels to get the day back. */
export const CANCEL_WARNING = 'Дневной лимит не возвращается: бесплатная бронь уже использована';

/** What a reservation that is being given back shows while the answer is on its way. */
export const CANCEL_PENDING = 'Отменяем бронь…';

/** What the panel says once the server confirmed the cancellation. */
export const CANCEL_CONFIRMED = 'Бронь отменена. Автомобиль снова доступен другим пользователям';

/** What the panel says about an account that is riding rather than reserving. */
export const RIDE_RUNNING = 'Аренда начата: поездка идёт';

/** The countdown at zero, while the server has not yet confirmed what happened. */
export const EXPIRY_PENDING = 'Срок брони истёк, проверяем состояние';

/** What is written before the time a reservation still has, wherever it is shown. */
export const TIME_LEFT = 'Осталось';

/** The heading of the warning that a reservation is running out. */
export const WARNING_HEADING = 'Бронь заканчивается';

/** What the warning says before the moment the reservation runs to. */
export const WARNING_ENDS_AT = 'Бронь закончится';

/** The one action of the warning, which marks it read on the server. */
export const READ_ACTION = 'Прочитано';

/** What the warning says while its read is on its way to the server. */
export const READ_PENDING = 'Отмечаем прочитанным…';

/** What the warning says when its read did not reach the server, so the person can ask again. */
export const READ_FAILED = 'Не удалось отметить прочитанным. Попробуйте ещё раз';

/** What the panel says when nothing is current and no reason was confirmed. */
export const NOTHING_CURRENT = 'Текущей брони нет';

/** What the interface says when the conditions of the reservation it shows came out differently. */
export const TARIFF_CHANGED = 'Тариф изменился';

/** What the interface says about a command whose outcome it does not know yet. */
export const UNKNOWN_COMMAND = 'Результат команды неизвестен: ответ не получен';

/** What the interface offers to settle an unknown outcome with the key the command was sent with. */
export const REPEAT_ACTION = 'Повторить команду';

/** What the interface says instead of offering a repeat past its window. */
export const REPEAT_EXPIRED = 'Повторить команду больше нельзя: прошло больше суток';

/** Where a person goes from the panel to the vehicle that is held for them. */
export const GO_TO_VEHICLE = 'Показать на карте';

/** What the booking control shows when the day's allowance has been spent. */
const LIMIT_SPENT = 'Бесплатная бронь использована. Следующая доступна';

/** What the booking control shows when the allowance could not be read at all. */
const LIMIT_UNKNOWN = 'Состояние бесплатной брони неизвестно';

/** What the interface says when the person is not signed in and cannot book at all. */
export const SIGN_IN_TO_BOOK = 'Войдите, чтобы забронировать автомобиль';

/** Russian wording for each refusal the reservation commands can answer with. */
export const REFUSAL_TEXT: Record<string, string> = {
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
  ORIGIN_NOT_ALLOWED: 'Запрос отклонён. Откройте приложение по обычному адресу',
  AUTHENTICATION_REQUIRED: 'Войдите, чтобы забронировать автомобиль',
  OUTSTANDING_INVOICE: 'Есть неоплаченный счёт: новая бронь недоступна',
  OUTSIDE_SERVICE_ZONE: 'Автомобиль вне зоны обслуживания: вернитесь в зону и завершите поездку',
  TELEMETRY_STALE: 'Положение автомобиля давно не подтверждалось: подождите и повторите',
  SERVICE_UNAVAILABLE: 'Сервис временно недоступен. Повторите попытку позже',
};

/** What a refusal the table does not name is explained with. */
export const UNEXPLAINED_REFUSAL = 'Сервис не выполнил команду. Повторите попытку позже';

/** The sentence one refusal code is shown with. */
export function refusalText(code: ApiError['code']): string {
  return REFUSAL_TEXT[code] ?? UNEXPLAINED_REFUSAL;
}

/**
 * What the booking control shows for one state of the day's allowance. An allowance that could not
 * be read is never presented as permission to book: the control is disabled and says so, and the
 * server decides the command again in any case.
 */
export function limitText(snapshot: CurrentSnapshot | undefined): string {
  if (snapshot === undefined) return LIMIT_UNKNOWN;
  if (snapshot.daily_limit.available) return BOOK_ACTION;

  return `${LIMIT_SPENT} ${bishkekMoment(snapshot.daily_limit.resets_at) ?? '—'}`;
}

/** Whether the allowance as published allows the control to be offered at all. */
export function limitAllowsBooking(snapshot: CurrentSnapshot | undefined): boolean {
  return snapshot?.daily_limit.available === true;
}

/**
 * bishkekMoment writes a moment of the contract in the timezone the service states its days in, so
 * a reset moment arrives as UTC and is read as the local date and time a person acts on. A moment
 * the interface cannot read is left out rather than guessed at.
 */
export function bishkekMoment(wireMoment: string): string | undefined {
  const moment = Date.parse(wireMoment);
  if (Number.isNaN(moment)) return undefined;

  return new Intl.DateTimeFormat('ru-RU', {
    timeZone: SERVICE_TIME_ZONE,
    day: 'numeric',
    month: 'long',
    hour: '2-digit',
    minute: '2-digit',
  }).format(moment);
}

/** The rates of one reservation, as the snapshot it was made under states them. */
export type RateText = { driving: string | null; paused: string | null };

/** What a rate is charged for, which is what makes two rates of the same rental comparable. */
export const RATE_UNIT = 'за начатую минуту';

/** What is written instead of the rates when the stored snapshot states none of them. */
export const TARIFF_MISSING = 'Тариф не указан';

/** The two rates of a stored snapshot, each written as a price or held out when unreadable. */
export function rateTextOf(snapshot: TariffSnapshot): RateText {
  return {
    driving: somText(snapshot.driving_rate_tyiyn_per_started_minute) ?? null,
    paused: somText(snapshot.paused_rate_tyiyn_per_started_minute) ?? null,
  };
}

/** Whether two published rates say the same thing, which is what "the tariff changed" is decided by. */
export function sameRates(left: RateText, right: RateText): boolean {
  return left.driving === right.driving && left.paused === right.paused;
}

/** The rental a current answer holds, or undefined when nothing is current. */
export function currentRental(snapshot: CurrentSnapshot | undefined): Rental | undefined {
  return snapshot?.kind === 'rental' ? snapshot.rental : undefined;
}

/** The model of the vehicle a rental holds, which is what a person recognises it by. */
export function vehicleName(rental: Rental): string {
  return rental.vehicle.model;
}

/** Everything the warning about a reservation that is running out says. */
export type WarningText = { vehicle: string; deadline: string; remaining: string };

/**
 * warningText writes the warning about one reservation: which vehicle is held, when the reservation
 * ends and how long is left. A countdown that is not there, that cannot be read or that has reached
 * the deadline is not a warning any more — the last minute is over — so nothing is written and
 * nothing is shown.
 */
export function warningText(
  warning: { vehicle: string; expiresAt: string },
  countdown: Countdown | undefined,
): WarningText | undefined {
  if (countdown === undefined || countdown.state !== 'left') return undefined;

  const endsAt = bishkekMoment(warning.expiresAt);
  if (endsAt === undefined) return undefined;

  return {
    vehicle: warning.vehicle,
    deadline: `${WARNING_ENDS_AT} ${endsAt}`,
    remaining: `${TIME_LEFT} ${countdown.text}`,
  };
}
