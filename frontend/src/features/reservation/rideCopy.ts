import type { Progress } from '../../shared/api/current.ts';
import { somText } from '../fleet/money.ts';
import type { RideMode } from './ridePace.ts';

/**
 * The Russian wording of a ride in force. A ride is a state a person watches rather than a step they
 * take, so what it says is the mode it is in and what the ride has cost so far.
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

/**
 * What the ride has cost so far, as the service stated it. The estimate is the one the service
 * computed from the rental's own rates, so the interface divides it exactly rather than estimating
 * it again; an amount it cannot read is written as missing rather than as zero.
 */
export function amountText(progress: Progress): string {
  return somText(progress.estimated_amount_tyiyn) ?? UNREADABLE_VALUE;
}
