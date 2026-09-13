import { INTERFACE_LOCALE } from '../../shared/locale.ts';

/** How many tyiyn make one som. Prices arrive as whole tyiyn and are shown as som. */
const TYIYN_IN_A_SOM = 100n;

/** Som are written with both minor digits, as 12,34 rather than 12,3. */
const MINOR_DIGITS = 2;

const somFormat = new Intl.NumberFormat(INTERFACE_LOCALE);

/** The decimal mark the interface locale writes, taken from the locale rather than assumed. */
const DECIMAL_MARK = somFormat.formatToParts(1.1).find((part) => part.type === 'decimal')?.value ?? ',';

/**
 * somText renders a price the service stated in whole tyiyn. The amount arrives as an exact decimal
 * string and is divided as an integer, because a price that passed through a floating-point number
 * would no longer be the price the operator set.
 *
 * A value that is not a whole, non-negative number of tyiyn is refused rather than repaired: the
 * interface shows the price the service published or says it has none.
 */
export function somText(tyiyn: string): string | undefined {
  const amount = wholeTyiyn(tyiyn);
  if (amount === undefined) return undefined;
  const minor = (amount % TYIYN_IN_A_SOM).toString().padStart(MINOR_DIGITS, '0');
  return `${somFormat.format(amount / TYIYN_IN_A_SOM)}${DECIMAL_MARK}${minor} сома`;
}

function wholeTyiyn(tyiyn: string): bigint | undefined {
  try {
    const amount = BigInt(tyiyn);
    return amount < 0n ? undefined : amount;
  } catch {
    return undefined;
  }
}
