import { useCallback, useEffect, useState } from 'react';
import type { CurrentSnapshot } from '../../shared/api/current.ts';
import { fetchInvoice, type InvoiceView } from '../../shared/api/invoices.ts';
import { loadedValue, type Resource } from '../../shared/api/Resource.ts';
import { useResource } from '../../shared/api/useResource.ts';
import { RECONCILE_MILLISECONDS } from '../events/reconciliation.ts';
import type { Notifications } from '../notifications/useNotifications.ts';
import { completedRide, type CompletedRide } from './completedRide.ts';
import {
  completedNotification,
  completedResult,
  invoiceWorthReading,
  shownResult,
  type CompletedRideResult,
} from './completedResult.ts';
import { currentRental } from './reservationCopy.ts';
import type { Payment } from './usePayment.ts';
import type { RideCommands } from './useRideCommands.ts';

/** Everything the result of a completed ride is read from, all of which the panel already holds. */
export type CompletedRideResultOptions = {
  /** The account the result belongs to, which the record this tab wrote is kept apart by. */
  owner: string | undefined;

  /** What the private read answered, which is what says no rental is current. */
  resource: Resource<CurrentSnapshot | undefined>;

  /** The commands of the ride, which is where a finish this tab sent is noticed. */
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
 * with nothing kept in this one; the record this tab wrote is only what the result is shown from
 * until the service's own notification has been read.
 *
 * The invoice is read again while its payment is one the service still owes an attempt — a state that
 * moves without this tab doing anything — and once more for a payment this tab sent, whose answer the
 * service has stored by the time it arrives. A state it has settled changes only when a person pays
 * again, which is a command rather than something to poll for.
 */
export function useCompletedRideResult(options: CompletedRideResultOptions): CompletedRideResult | undefined {
  const { owner, resource, ride, notifications, paid } = options;

  const record = useRecordedRide(owner, resource, ride);
  const published = usePublishedResult(notifications, paid);

  return shownResult(published, record);
}

/**
 * useRecordedRide reads the ending this tab ended itself, from the record its finish handler wrote.
 * The record is read when the account changes and whenever a rental stops being current, so a ride
 * this tab ended is shown on the answer that ended it rather than one reconciliation later.
 */
function useRecordedRide(
  owner: string | undefined,
  resource: Resource<CurrentSnapshot | undefined>,
  ride: RideCommands,
): CompletedRide | undefined {
  const [record, setRecord] = useState<CompletedRide | undefined>(undefined);

  const rentalId = currentRental(loadedValue(resource))?.id;
  const settled = ride.phase.state === 'done' && ride.phase.action === 'finish';
  useEffect(() => {
    setRecord(completedRide(owner, Date.now()));
  }, [owner, resource, rentalId, settled]);

  return record;
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

  // A payment this tab sent has been stored by the service by the time its answer arrives, so the
  // invoice is read again rather than left showing the state that payment replaced.
  const answered = paid.phase.state === 'done' && paid.phase.action === 'pay';
  useEffect(() => {
    if (answered) retry();
  }, [answered, retry]);

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
