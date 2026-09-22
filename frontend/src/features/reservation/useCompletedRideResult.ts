import { useCallback, useState } from 'react';
import { fetchInvoice, type InvoiceView } from '../../shared/api/invoices.ts';
import type { Payment } from '../../shared/api/current.ts';
import { loadedValue } from '../../shared/read/Resource.ts';
import { RECONCILE_MILLISECONDS } from '../../shared/read/reconciliation.ts';
import { useResource } from '../../shared/read/useResource.ts';
import { useReadAfterPayment } from '../../shared/command/useReadAfterPayment.ts';
import type { Payment as PaymentCommand } from '../../shared/command/usePayment.ts';
import type { Notifications } from '../../shared/account/notifications.ts';
import {
  completedNotification,
  completedResult,
  invoiceWorthReading,
  shownResult,
  type CompletedRideResult,
} from './completedResult.ts';
import type { RideCommands } from './useRideCommands.ts';

/** Everything the result of a completed ride is read from, all of which the panel already holds. */
export type CompletedRideResultOptions = {
  /** The commands of the ride, which hold what the service answered a finish this tab sent. */
  ride: RideCommands;

  /** The notifications addressed to the account, which the completion is read from. */
  notifications: Notifications;

  /** Where the payment this tab sent last stands, which is what one more read follows. */
  paid: PaymentCommand;
};

/** One read of the invoice that a completion names, which is what its state is taken from. */
type Invoiced = { invoiceId: string | undefined; payment: Payment | undefined };

/**
 * useCompletedRideResult produces what the panel shows while the account has no current rental: the
 * result of the ride that ended last. It is read from the notifications the service holds for the
 * account and from the invoice the completion names, so a reload and a new tab show the same result
 * with nothing kept in this one; the answer to a finish this tab sent is what the result is shown
 * from until the service's own report of that ending has been read, and it is what names the vehicle.
 *
 * The invoice is read again while its payment is one the service still owes an attempt — a state that
 * moves without this tab doing anything — and once more for a payment this tab sent, whose answer the
 * service has stored by the time it arrives. A state it has settled changes only when a person pays
 * again, which is a command rather than something to poll for.
 *
 * Whether there is anything left to wait for is stated by the payment the invoice publishes, which is
 * known only once it has been read. That reading is what decides the interval, so the two are the
 * same value rather than one render apart.
 */
export function useCompletedRideResult(options: CompletedRideResultOptions): CompletedRideResult | undefined {
  const { ride, notifications, paid } = options;

  return shownResult(usePublishedResult(notifications, paid), ride.finished);
}

/**
 * usePublishedResult reads what the service holds about the last ride that ended: the newest
 * notification of that kind, and the invoice that notification names.
 */
function usePublishedResult(notifications: Notifications, paid: PaymentCommand): CompletedRideResult | undefined {
  const notification = completedNotification(notifications.reading?.collection);
  const invoiceId = notification?.invoice_id;

  const [invoiced, setInvoiced] = useState<Invoiced>(() => invoicedFor(invoiceId));
  // A read belongs to the invoice it was made about: another invoice is another state of payment, and
  // the answer to the previous one cannot say anything about it.
  const current = invoiced.invoiceId === invoiceId ? invoiced : invoicedFor(invoiceId);

  const load = useCallback(
    (signal: AbortSignal) => readInvoice(invoiceId, signal).then((view) => keep(setInvoiced, invoiceId, view)),
    [invoiceId],
  );
  const handle = useResource<InvoiceView | undefined>(load, undefined, intervalOf(invoiceId, current.payment));

  const published = completedResult(notification, loadedValue(handle.resource));
  useReadAfterPayment(paid, handle.retry);

  return published;
}

/** What is known about an invoice nothing has been read into yet: nothing at all. */
function invoicedFor(invoiceId: string | undefined): Invoiced {
  return { invoiceId, payment: undefined };
}

/** Keeps the answer of one read, which is what the interval of the next one is computed from. */
function keep(
  setInvoiced: (update: (held: Invoiced) => Invoiced) => void,
  invoiceId: string | undefined,
  view: InvoiceView | undefined,
): InvoiceView | undefined {
  const payment = view?.payment;
  setInvoiced((held) => (held.invoiceId === invoiceId && held.payment === payment ? held : { invoiceId, payment }));

  return view;
}

/**
 * How often the invoice is read again, or nothing when there is no invoice to read or nothing left to
 * wait for. The interval is the one reconciliation runs on: an invoice is another resource that the
 * service moves on its own, and a second interval would be a second answer to how often to look.
 */
function intervalOf(invoiceId: string | undefined, payment: Payment | undefined): number | undefined {
  if (invoiceId === undefined) return undefined;
  if (!invoiceWorthReading(payment)) return undefined;

  return RECONCILE_MILLISECONDS;
}

/** The invoice one notification names, or nothing when no notification names one. */
function readInvoice(invoiceId: string | undefined, signal: AbortSignal): Promise<InvoiceView | undefined> {
  if (invoiceId === undefined) return Promise.resolve(undefined);

  return fetchInvoice(invoiceId, signal);
}
