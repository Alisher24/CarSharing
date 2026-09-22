import type { Completion, TariffSnapshot } from '../api/current.ts';
import { somText } from '../money.ts';
import { RIDE_MODE_TEXT } from './pace.ts';

/**
 * What one rental costs, and how a price is written: the two rates a tariff snapshot stores, and the
 * rule that holds out a rate or an amount the interface cannot read rather than repairing it. It is
 * declared apart from the wording of a ride because the card that books a vehicle, the panel of the
 * rental in force and the invoice of a finished ride all price the same rental from it.
 */

/**
 * What is written where the answer carries no value the interface can read. A missing rate, a missing
 * amount and a missing moment are missing in the same way, so one mark says all three.
 */
export const UNREADABLE_VALUE = '—';

/** What a rate is charged for, which is what makes two rates of the same rental comparable. */
export const RATE_UNIT = 'за начатую минуту';

/** What is written instead of the rates when the stored snapshot states none of them. */
export const TARIFF_MISSING = 'Тариф не указан';

/** The rates of one rental, as the snapshot it was made under states them. */
export type RateText = { driving: string | null; paused: string | null };

/** The rates of one rental as prices, which is what a view may show. */
export type PricedRates = { driving: string; paused: string };

/** The two rates of a stored snapshot, each written as a price or held out when unreadable. */
export function rateTextOf(snapshot: TariffSnapshot): RateText {
  return {
    driving: somText(snapshot.driving_rate_tyiyn_per_started_minute) ?? null,
    paused: somText(snapshot.paused_rate_tyiyn_per_started_minute) ?? null,
  };
}

/**
 * The rates of one rental as prices, or undefined when either of them could not be read as one. A rate
 * is held out rather than repaired, and one unreadable rate is enough: a rental that states one price
 * alone cannot say what it costs, and a made-up price is worse than a missing one.
 */
export function pricedRates(rates: RateText): PricedRates | undefined {
  if (rates.driving === null || rates.paused === null) return undefined;

  return { driving: rates.driving, paused: rates.paused };
}

/** Whether two published rates say the same thing, which is what "the tariff changed" is decided by. */
export function sameRates(left: RateText, right: RateText): boolean {
  return left.driving === right.driving && left.paused === right.paused;
}

/** The rate rows of one rental, in the order a view lists them, each named as the mode it prices. */
export const RATE_ROWS: readonly { mode: keyof RateText; title: string }[] = [
  { mode: 'driving', title: RIDE_MODE_TEXT.driving },
  { mode: 'paused', title: RIDE_MODE_TEXT.paused },
];

/**
 * One amount of the contract as a person reads it. The amount is the one the service published — the
 * estimate of a ride in force, or the total of an invoice it issued — and nothing here adds anything
 * up: an interface that computed its own total could disagree with what a person is charged. An
 * amount it cannot read is written as missing rather than as zero.
 */
export function amountText(tyiyn: string): string {
  return somText(tyiyn) ?? UNREADABLE_VALUE;
}

/** What each completion reason is called, in the words a person reads. */
export const COMPLETION_TEXT: Record<Completion['reason'], string> = {
  user_finished: 'поездку завершил пользователь',
  energy_depleted: 'закончился запас энергии или топлива',
};

/** What a completion reason the interface does not know is written as. */
export const COMPLETION_UNKNOWN = 'причина не указана';
