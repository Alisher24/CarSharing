import type { InvoiceLine, InvoiceView } from '../../shared/api/current.ts';
import { INTERFACE_LOCALE, datedServiceMoment } from '../../shared/locale.ts';
import { somText } from '../fleet/money.ts';
import { paymentText } from '../reservation/paymentCopy.ts';
import { rideModeText } from '../reservation/ridePace.ts';
import { RATE_UNIT } from '../reservation/reservationCopy.ts';
import { UNREADABLE_VALUE } from '../reservation/rideCopy.ts';

/**
 * What a person reads about an invoice: one row of the feed, and the lines of the card that opens
 * from it. The card is the one place the interface states what the letter states, which is what
 * makes the two comparable: the minutes begun in each mode, the rate they were priced at, what each
 * of them cost and the total of the two.
 *
 * Nothing here adds an amount up. Every number is the one the service published, rendered in the
 * interface locale, and a number that cannot be read is written as missing rather than as zero.
 */

/** What one row of the invoice feed states. */
export type InvoiceRow = {
  /** The invoice the row is about, which is what a list keys it by and what the row links to. */
  id: string;

  issuedAt: string;
  total: string;
  payment: string;
};

/** What one line of the card states: the mode it describes, and what that mode was charged at. */
export type InvoiceLineRow = {
  mode: string;
  minutes: string;
  rate: string;
  amount: string;
};

/** What is written before each value of a row or of a line. */
export const INVOICE_ISSUED_AT = 'Выставлен';
export const INVOICE_TOTAL = 'Итог';
export const INVOICE_PAYMENT = 'Оплата';
export const LINE_MINUTES = 'Начатых минут';
export const LINE_RATE = 'Ставка';
export const LINE_AMOUNT = 'Сумма';

const wholeNumberFormat = new Intl.NumberFormat(INTERFACE_LOCALE);

/** invoiceRow builds what one row of the feed states from the view the service published. */
export function invoiceRow(view: InvoiceView): InvoiceRow {
  return {
    id: view.invoice.id,
    issuedAt: datedServiceMoment(view.invoice.issued_at) ?? UNREADABLE_VALUE,
    total: somText(view.invoice.total_amount_tyiyn) ?? UNREADABLE_VALUE,
    payment: paymentText(view.payment),
  };
}

/** invoiceLineRows builds the lines of the card, in the order the invoice publishes them. */
export function invoiceLineRows(view: InvoiceView): readonly InvoiceLineRow[] {
  return view.invoice.lines.map(invoiceLineRow);
}

function invoiceLineRow(line: InvoiceLine): InvoiceLineRow {
  return {
    mode: rideModeText(line.mode),
    minutes: wholeNumber(line.billed_started_minutes),
    rate: rateText(line.rate_tyiyn_per_started_minute),
    amount: somText(line.amount_tyiyn) ?? UNREADABLE_VALUE,
  };
}

/** A rate as the price of one started minute, which is what the invoice charges by. */
function rateText(tyiyn: string): string {
  const price = somText(tyiyn);
  return price === undefined ? UNREADABLE_VALUE : `${price} ${RATE_UNIT}`;
}

/**
 * A whole number the contract states as an exact decimal string. It is read as an integer rather
 * than as a number, because a count that passed through a floating-point value would no longer be
 * the count the service charged for.
 */
function wholeNumber(exact: string): string {
  try {
    return wholeNumberFormat.format(BigInt(exact));
  } catch {
    return UNREADABLE_VALUE;
  }
}
