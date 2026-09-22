// How an amount of the contract is written for a person: whole tyiyn, both minor digits, and the unit
// the interface writes. It is the rule `frontend/src/shared/money.ts` applies, restated here because a
// browser check compares a letter and a screen against an invoice the service published, and a check
// that read the expected text out of the thing it checks would prove nothing.
//
// The two parts of the rule a check cannot derive from the amount it is checking — the locale, and the
// unit an amount is written with — are read from that declaration by `scripts/money-declaration.test.mjs`
// rather than assumed: a change to either is a change here, not a check that compares against a unit
// nobody writes.
const INTERFACE_LOCALE = 'ru-RU';

/** How many tyiyn make one som. */
const TYIYN_IN_SOM = 100n;

/** Som are written with both minor digits, as `12,34` rather than `12,3`. */
const MINOR_DIGITS = 2;

/** The unit the interface writes an amount with, which is the som and not `KGS`. */
const SOM_UNIT = 'сома';

const somFormat = new Intl.NumberFormat(INTERFACE_LOCALE);
const DECIMAL_MARK = somFormat.formatToParts(1.1).find((part) => part.type === 'decimal')?.value ?? ',';

/** One amount of whole tyiyn written the way a person reads it. */
export function somText(tyiyn) {
  const amount = BigInt(tyiyn);
  const minor = (amount % TYIYN_IN_SOM).toString().padStart(MINOR_DIGITS, '0');
  return `${somFormat.format(amount / TYIYN_IN_SOM)}${DECIMAL_MARK}${minor} ${SOM_UNIT}`;
}
