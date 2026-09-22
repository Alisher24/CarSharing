import type { CurrentSnapshot } from '../../shared/api/current.ts';
import { serviceMoment } from '../../shared/locale.ts';
import { BOOK_ACTION } from '../../shared/copy.ts';
import type { Countdown } from '../../shared/ride/countdown.ts';
import { TIME_LEFT } from '../../shared/ride/spell.ts';

/**
 * The Russian wording of a reservation, and the rules that produce it: what the day's allowance is
 * called, what the confirmation says, and what the panel reports about the reservation in force. It
 * lives here rather than in each component that shows it, so the panel and the card cannot describe
 * one state two ways.
 *
 * What a ride is written with is not here: a ride outlives the reservation that started it and is
 * read by the cabinet as well, so those words live beside the ride.
 */

/** The heading of the panel that shows the reservation in force. */
export const PANEL_HEADING = 'Текущая бронь';

/** What the panel offers while a reservation is running. */
export const CANCEL_ACTION = 'Отменить бронь';

/** What cancelling a reservation asks, in the words the specification fixes. */
export const CANCEL_QUESTION = 'Отменить бронь? Автомобиль станет доступен другим пользователям';

/** What the cancellation keeps warning about, so nobody cancels to get the day back. */
export const CANCEL_WARNING = 'Дневной лимит не возвращается: бесплатная бронь уже использована';

/** What the panel says once the server confirmed the cancellation. */
export const CANCEL_CONFIRMED = 'Бронь отменена. Автомобиль снова доступен другим пользователям';

/** The countdown at zero, while the server has not yet confirmed what happened. */
export const EXPIRY_PENDING = 'Срок брони истёк, проверяем состояние';

/** Where a person goes from the panel to the vehicle that is held for them. */
export const GO_TO_VEHICLE = 'Показать на карте';

/** What the interface says when the conditions of the reservation it shows came out differently. */
export const TARIFF_CHANGED = 'Тариф изменился';

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

/** What the booking control shows when the day's allowance has been spent. */
const LIMIT_SPENT = 'Бесплатная бронь использована. Следующая доступна';

/** What the booking control shows when the allowance could not be read at all. */
const LIMIT_UNKNOWN = 'Состояние бесплатной брони неизвестно';

/**
 * What the booking control shows for one state of the day's allowance. An allowance that could not
 * be read is never presented as permission to book: the control is disabled and says so, and the
 * server decides the command again in any case.
 */
export function limitText(snapshot: CurrentSnapshot | undefined): string {
  if (snapshot === undefined) return LIMIT_UNKNOWN;
  if (snapshot.daily_limit.available) return BOOK_ACTION;

  return `${LIMIT_SPENT} ${serviceMoment(snapshot.daily_limit.resets_at) ?? '—'}`;
}

/** Whether the allowance as published allows the control to be offered at all. */
export function limitAllowsBooking(snapshot: CurrentSnapshot | undefined): boolean {
  return snapshot?.daily_limit.available === true;
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

  const endsAt = serviceMoment(warning.expiresAt);
  if (endsAt === undefined) return undefined;

  return {
    vehicle: warning.vehicle,
    deadline: `${WARNING_ENDS_AT} ${endsAt}`,
    remaining: `${TIME_LEFT} ${countdown.text}`,
  };
}
