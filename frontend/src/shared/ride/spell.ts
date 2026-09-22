import type { Completion, Rental } from '../api/current.ts';
import { serviceMoment } from '../locale.ts';
import { sourceName } from '../vehicle/spell.ts';
import { COMPLETION_TEXT, COMPLETION_UNKNOWN, UNREADABLE_VALUE } from './fares.ts';

/**
 * The Russian wording a ride is written with, declared once for the three views that show one: the
 * panel of the rental in force, the card that books a vehicle and the cabinet that reads the history
 * back. What one view alone shows stays in that view.
 */

/** What the control of a reservation offers, which is the one command that starts the ride. */
export const START_ACTION = 'Начать поездку';

/** What the control of a moving ride offers. */
export const PAUSE_ACTION = 'Пауза';

/** What the control of a held ride offers. */
export const RESUME_ACTION = 'Продолжить';

/** What the control that ends the ride offers. */
export const FINISH_ACTION = 'Завершить поездку';

/** What ending a ride asks before it is sent, because an ending is not undone by asking again. */
export const FINISH_QUESTION = 'Завершить поездку?';

/** What the question warns about: the invoice is issued for what the ride has taken so far. */
export const FINISH_WARNING = 'Счёт будет выставлен за время поездки и не изменится после завершения';

/** What the control that keeps the ride going offers, instead of ending it. */
export const KEEP_RIDING_ACTION = 'Продолжить поездку';

/** What the panel says once the server confirmed that the ride is over. */
export const RIDE_FINISHED = 'Поездка завершена';

/** What the panel says when the account has nothing current and no reason was confirmed. */
export const NOTHING_CURRENT = 'Текущей брони нет';

/** What is written before the time a reservation still has, wherever it is shown. */
export const TIME_LEFT = 'Осталось';

/** What is written before the time a ride has spent in its current mode. */
export const IN_MODE = 'В режиме';

/** What is written before how long the ride has moved in total. */
export const DRIVING_TOTAL = 'Всего в движении';

/** What is written before how long the ride has been held in total. */
export const PAUSED_TOTAL = 'Всего в паузе';

/** What is written before what the ride has cost so far. */
export const ESTIMATED_AMOUNT = 'Оценка стоимости';

/** What is written before the total of the invoice a ride produced. */
export const INVOICE_TOTAL = 'Итог счёта';

/** What is written before why a ride ended, wherever the ending is reported. */
export const COMPLETION_REASON = 'Причина';

/** What is written before the moment a ride ended. */
export const FINISHED_AT = 'Завершена';

/**
 * Why a ride ended, in the words the interface shows for the completion the contract carries. An
 * ending the vehicle caused names the sources that ran out, in the words the fleet names them by:
 * which of them was empty is what a person can act on, and the reason alone does not say it.
 */
export function completionText(completion: Completion): string {
  const reason = COMPLETION_TEXT[completion.reason] ?? COMPLETION_UNKNOWN;
  const sources = exhaustedText(completion);
  if (sources === undefined) return reason;

  return `${reason}: ${sources}`;
}

/**
 * The sources that ran out, listed, or undefined when the ending is not one an empty vehicle caused.
 * The contract allows the list to be empty, and then there is nothing to add to the reason.
 */
function exhaustedText(completion: Completion): string | undefined {
  if (completion.reason !== 'energy_depleted') return undefined;
  if (completion.exhausted_sources.length === 0) return undefined;

  return completion.exhausted_sources.map(sourceName).join(', ');
}

/** The vehicle a rental holds, which is what a person recognises the rental by. */
export function vehicleName(rental: Rental): string {
  return rental.vehicle.model;
}

/** The moment a ride ended, in the time zone the service states its days in. */
export function finishedAtText(endedAt: string): string {
  return serviceMoment(endedAt) ?? UNREADABLE_VALUE;
}
