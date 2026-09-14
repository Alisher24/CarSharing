import type { FinishResult, InvoiceView } from '../../shared/api/current.ts';

/**
 * What the browser remembers about a ride it ended: the answer the service gave, which is the invoice
 * of that ride together with the ride it ended.
 *
 * The service publishes no shape for "the ride I finished last" — that is the account screen of a
 * later task — and a finished ride is no longer current, so the panel would otherwise lose the amount
 * the moment the ending is confirmed. The answer is therefore kept where the handler that received it
 * wrote it, for the tab it was received in, and is cleared as soon as the account starts something
 * new. A reload of another tab, or of this one after its storage is cleared, shows no result rather
 * than a wrong one.
 */

/** One stored ending: whose ride it was and what the service answered. */
export type CompletedRide = {
  /** The account the ending belongs to, so it is never shown to another one. */
  owner: string;

  /** The moment the answer was received here, which is what its window is measured from. */
  receivedAt: number;

  /** The answer itself, as the service gave it. */
  finished: FinishResult;

  /**
   * The state of what is owed on the invoice, as the service last published it: the answer that ended
   * the ride, and the answer to every payment since. It is kept beside the ending rather than derived
   * from it, because the payment moves while the invoice and the ride never do.
   *
   * Nothing else in this build publishes that state: reading an invoice from the service is a later
   * task, and the signal that a payment changed carries a version but not a status.
   */
  payment: InvoiceView;
};

/** How long an ending is shown. Past it the account screen of a later task is where it belongs. */
export const COMPLETED_RIDE_MILLISECONDS = 24 * 60 * 60 * 1_000;

/** Where the record is kept: the storage of one tab, cleared when that tab is closed. */
const STORAGE_KEY = 'carsharing.completed-ride';

/** The part of a storage this module uses, so a test can supply its own. */
export type CompletionStorage = {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
  removeItem: (key: string) => void;
};

/**
 * storeCompletedRide records the answer a finish produced. A browser that cannot write it — a private
 * window with storage disabled — is not an error: the ending happened, and only the ability to show
 * its result after a reload is lost.
 */
export function storeCompletedRide(
  owner: string,
  finished: FinishResult,
  receivedAt: number,
  storage = browserStorage(),
): void {
  writeCompletedRide({ owner, receivedAt, finished, payment: finished.invoice }, storage);
}

/**
 * storePayment records the state of a payment the service has just published, in the record of the
 * ending it belongs to. It is written by the handler that received the answer rather than by a render,
 * so a reload of the tab shows the state the service confirmed instead of the one it replaced.
 *
 * A record that is not this account's is not touched: another account's ending is not this payment's
 * to describe, and a record that cannot be read is left as it is rather than replaced by a payment
 * with no invoice beside it.
 */
export function storePayment(
  owner: string,
  invoiceId: string,
  payment: InvoiceView,
  receivedAt: number,
  storage = browserStorage(),
): void {
  const held = readCompletedRide(storage);
  if (held === undefined || held.owner !== owner) return;
  if (held.finished.invoice.invoice.id !== invoiceId) return;

  writeCompletedRide({ ...held, receivedAt, payment }, storage);
}

function writeCompletedRide(record: CompletedRide, storage: CompletionStorage | undefined): void {
  try {
    storage?.setItem(STORAGE_KEY, JSON.stringify(record));
  } catch {
    // Storage that refuses the write costs the summary on screen, never the ending itself.
  }
}

/** clearCompletedRide forgets the ending, which the account does when it starts something new. */
export function clearCompletedRide(storage = browserStorage()): void {
  try {
    storage?.removeItem(STORAGE_KEY);
  } catch {
    // A record that cannot be removed is stale, not dangerous: it is refused below.
  }
}

/**
 * completedRide answers the ending this account may still be shown, or undefined when there is none.
 * A record of another account, a record past its window and a record that cannot be read are all
 * answered as no ending at all: the panel then shows what the service says about the account, which
 * is nothing rather than something it cannot stand behind.
 */
export function completedRide(
  owner: string | undefined,
  now: number,
  storage = browserStorage(),
): CompletedRide | undefined {
  if (owner === undefined) return undefined;

  const held = readCompletedRide(storage);
  if (held === undefined) return undefined;
  if (held.owner !== owner) return undefined;
  if (now - held.receivedAt >= COMPLETED_RIDE_MILLISECONDS) return undefined;

  return held;
}

function readCompletedRide(storage: CompletionStorage | undefined): CompletedRide | undefined {
  let raw: string | null = null;
  try {
    raw = storage?.getItem(STORAGE_KEY) ?? null;
  } catch {
    return undefined;
  }
  if (raw === null) return undefined;

  try {
    const parsed = JSON.parse(raw) as CompletedRide;
    if (typeof parsed?.owner !== 'string') return undefined;
    if (typeof parsed.receivedAt !== 'number') return undefined;
    if (!isFinishedResult(parsed.finished)) return undefined;
    // A record written before payments were kept states none of its own. The view the ending carried
    // is the state it was written with, which is what the panel shows rather than nothing at all.
    if (!isInvoiceView(parsed.payment)) parsed.payment = parsed.finished.invoice;

    return parsed;
  } catch {
    return undefined;
  }
}

/**
 * isFinishedResult reports whether a stored value is still the answer a finish gives. A record written
 * by another build is refused here rather than read into a render that would show blanks: the two
 * fields the summary is made of are what has to be there.
 */
function isFinishedResult(finished: unknown): finished is FinishResult {
  if (typeof finished !== 'object' || finished === null) return false;
  const answer = finished as FinishResult;
  if (typeof answer.rental?.state !== 'string') return false;
  if (typeof answer.rental?.vehicle?.model !== 'string') return false;
  if (typeof answer.invoice?.invoice?.total_amount_tyiyn !== 'string') return false;

  return typeof answer.rental?.completion?.reason === 'string';
}

/**
 * isInvoiceView reports whether a stored value is still a view of an invoice. The status is what the
 * payment is shown by, so a record that does not state one is refused: a payment drawn from a value
 * that cannot say what it is would be a state nobody published.
 */
function isInvoiceView(payment: unknown): payment is InvoiceView {
  if (typeof payment !== 'object' || payment === null) return false;
  const view = payment as InvoiceView;
  if (typeof view.invoice?.id !== 'string') return false;
  if (typeof view.version !== 'string') return false;

  return typeof view.payment?.status === 'string';
}

function browserStorage(): CompletionStorage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}
