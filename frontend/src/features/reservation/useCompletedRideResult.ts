import { useCallback, useEffect, useState } from 'react';
import { fetchInvoice, type InvoiceView } from '../../shared/api/invoices.ts';
import { loadedValue } from '../../shared/api/Resource.ts';
import { useResource } from '../../shared/api/useResource.ts';
import { RECONCILE_MILLISECONDS } from '../events/reconciliation.ts';
import type { Notifications } from '../notifications/useNotifications.ts';
import {
  completedNotification,
  completedResult,
  invoiceWorthReading,
  shownResult,
  type CompletedRideResult,
} from './completedResult.ts';
import { useReadAfterPayment, type Payment } from './usePayment.ts';
import type { RideCommands } from './useRideCommands.ts';

/** Everything the result of a completed ride is read from, all of which the panel already holds. */
export type CompletedRideResultOptions = {
  /** The commands of the ride, which hold what the service answered a finish this tab sent. */
  ride: RideCommands;

  /** The notifications addressed to the account, which the completion is read from. */
  notifications: Notifications;

  /** Where the payment this tab sent last stands, which is what one more read follows. */
  paid: Payment;
};

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
 */
export function useCompletedRideResult(options: CompletedRideResultOptions): CompletedRideResult | undefined {
  const { ride, notifications, paid } = options;

  return shownResult(usePublishedResult(notifications, paid), ride.finished);
}

/**
 * usePublishedResult reads what the service holds about the last ride that ended: the newest
 * notification of that kind, and the invoice that notification names.
 */
function usePublishedResult(notifications: Notifications, paid: Payment): CompletedRideResult | undefined {
  const notification = completedNotification(notifications.reading?.collection);
  const invoiceId = notification?.invoice_id;

  // Whether the invoice is worth reading again is stated by the payment it publishes, which is known
  // only once it has been read: the interval is therefore turned off by the answer that settles the
  // payment rather than by the request that asks for it, and that answer arrives one render later.
  const [waiting, setWaiting] = useState(true);
  const load = useCallback((signal: AbortSignal) => readInvoice(invoiceId, signal), [invoiceId]);
  const { resource, retry } = useResource<InvoiceView | undefined>(load, undefined, intervalOf(invoiceId, waiting));

  const published = completedResult(notification, loadedValue(resource));

  const moving = invoiceWorthReading(published?.charge?.payment);
  useEffect(() => {
    setWaiting(moving);
  }, [moving]);

  useReadAfterPayment(paid, retry);

  return published;
}

/**
 * How often the invoice is read again, or nothing when there is no invoice to read or nothing left to
 * wait for. The interval is the one reconciliation runs on: an invoice is another resource that the
 * service moves on its own, and a second interval would be a second answer to how often to look.
 */
function intervalOf(invoiceId: string | undefined, waiting: boolean): number | undefined {
  if (invoiceId === undefined) return undefined;
  if (!waiting) return undefined;

  return RECONCILE_MILLISECONDS;
}

/** The invoice one notification names, or nothing when no notification names one. */
function readInvoice(invoiceId: string | undefined, signal: AbortSignal): Promise<InvoiceView | undefined> {
  if (invoiceId === undefined) return Promise.resolve(undefined);

  return fetchInvoice(invoiceId, signal);
}
