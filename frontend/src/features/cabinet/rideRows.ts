import type { Completion } from '../../shared/api/current.ts';
import type { RideSummary } from '../../shared/api/history.ts';
import { datedServiceMoment } from '../../shared/locale.ts';
import { UNREADABLE_VALUE } from '../reservation/rideCopy.ts';

/**
 * What one row of the ride feed states: the vehicle the ride was taken on, the moments it ran
 * between, and why it ended. The feed answers "when and on what did I ride"; what the ride cost is
 * the invoice's answer, and the sources an empty vehicle ran out of are the invoice card's.
 */
export type RideRow = {
  /** The ride the row is about, which is what a list keys it by. */
  id: string;

  vehicle: string;
  startedAt: string;
  completedAt: string;
  reason: string;

  /** The invoice the ride was charged by, which the row links to. */
  invoiceId: string;
};

/** What is written before each value of a row. The vehicle needs none: it heads the row. */
export const RIDE_STARTED_AT = 'Начало';
export const RIDE_COMPLETED_AT = 'Окончание';
export const RIDE_REASON = 'Причина';

/**
 * Why a ride ended, in one word. The feed has room for the reason and not for the story: an ending
 * an empty vehicle caused names the sources it ran out of on the card of its invoice, which is where
 * a person goes to see what it cost them.
 */
const REASON_WORD: Record<string, string> = {
  user_finished: 'Вручную',
  energy_depleted: 'Исчерпание',
};

/** What a reason this build does not know is written as, rather than as one it does. */
export const REASON_UNKNOWN = 'Неизвестна';

/** rideRow builds what one row of the feed states from the summary the service published. */
export function rideRow(ride: RideSummary): RideRow {
  return {
    id: ride.id,
    vehicle: ride.vehicle.model,
    startedAt: datedServiceMoment(ride.started_at) ?? UNREADABLE_VALUE,
    completedAt: datedServiceMoment(ride.completed_at) ?? UNREADABLE_VALUE,
    reason: reasonWord(ride.completion),
    invoiceId: ride.invoice_id,
  };
}

function reasonWord(completion: Completion): string {
  return REASON_WORD[completion.reason] ?? REASON_UNKNOWN;
}
