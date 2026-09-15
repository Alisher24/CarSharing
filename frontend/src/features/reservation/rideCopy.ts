import type { Completion, Progress } from '../../shared/api/current.ts';
import { sourceName } from '../fleet/fleetCopy.ts';
import { somText } from '../fleet/money.ts';
import { serviceMoment } from '../../shared/locale.ts';
import type { RideMode } from './ridePace.ts';

/**
 * The Russian wording of a ride in force and of the ending of one. A ride is a state a person watches
 * rather than a step they take, so what it says is the mode it is in and what the ride has cost so
 * far; the ending is a decision, so it is asked and then reported.
 */

/** What each mode of a ride is called. */
export const PACE_TEXT: Record<RideMode, string> = {
  driving: 'Движение',
  paused: 'Пауза',
};

/** What is written before the time a ride has spent in its current mode. */
export const IN_MODE = 'В режиме';

/** What is written before how long the ride has moved in total. */
export const DRIVING_TOTAL = 'Всего в движении';

/** What is written before how long the ride has been held in total. */
export const PAUSED_TOTAL = 'Всего в паузе';

/** What is written before what the ride has cost so far. */
export const ESTIMATED_AMOUNT = 'Оценка стоимости';

/** What is written where a duration or an amount could not be read from the answer. */
export const UNREADABLE_VALUE = '—';

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

/** What is written before the total of the invoice a finished ride produced. */
export const INVOICE_TOTAL = 'Итог счёта';

/** What is written before why the ride ended. */
export const COMPLETION_REASON = 'Причина';

/** What is written before the moment the ride ended. */
export const FINISHED_AT = 'Завершена';

/** What each completion reason is called, in the words a person reads. */
export const COMPLETION_TEXT: Record<string, string> = {
  user_finished: 'поездку завершил пользователь',
  energy_depleted: 'закончился запас энергии или топлива',
};

/** What a completion reason the interface does not know is written as. */
export const COMPLETION_UNKNOWN = 'причина не указана';

/**
 * What the ride has cost so far, as the service stated it. The estimate is the one the service
 * computed from the rental's own rates, so the interface divides it exactly rather than estimating
 * it again; an amount it cannot read is written as missing rather than as zero.
 */
export function amountText(progress: Progress): string {
  return somText(progress.estimated_amount_tyiyn) ?? UNREADABLE_VALUE;
}

/**
 * What a completed ride cost, as the invoice the service issued states it. The total is the one the
 * invoice publishes rather than the sum of its lines computed here: an interface that added them up
 * itself could disagree with the amount a person is charged.
 */
export function invoiceAmountText(totalAmountTyiyn: string): string {
  return somText(totalAmountTyiyn) ?? UNREADABLE_VALUE;
}

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

/** The moment a ride ended, in the time zone the service states its days in. */
export function finishedAtText(endedAt: string): string {
  return serviceMoment(endedAt) ?? UNREADABLE_VALUE;
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
