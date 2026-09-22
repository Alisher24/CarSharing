import { INTERFACE_LOCALE } from './locale.ts';

/**
 * The moment a reading was taken, written to the second. It is shown in the reader's own zone,
 * because it answers "how fresh is this", not "at what time in Bishkek".
 */
const clockFormat = new Intl.DateTimeFormat(INTERFACE_LOCALE, {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
});

/** clockMoment writes one moment as a clock reading, by the same rule everywhere it is shown. */
export function clockMoment(moment: Date | number): string {
  return clockFormat.format(moment);
}
