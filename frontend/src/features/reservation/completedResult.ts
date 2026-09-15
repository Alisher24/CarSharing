import type { Completion, InvoiceView, Payment } from '../../shared/api/current.ts';
import type {
  Notification,
  NotificationCollection,
  RentalCompletedNotification,
} from '../../shared/api/notifications.ts';
import type { CompletedRide } from './completedRide.ts';

/**
 * What a person is shown about a ride that ended: why it ended, the moment it actually ended, what it
 * cost and where paying for it stands.
 *
 * The result is read from the account's own notifications and from the invoice the completion names,
 * so it is the same on the screen that ended the ride, after a reload and in another tab. The record
 * this tab wrote is only what the result is shown from before the service's own notification has been
 * read, and the vehicle is named by that record alone.
 */

/** What one completed ride cost and how it is being paid for, as the invoice issued for it states. */
export type CompletedRideCharge = {
  /** The invoice this charge was read from, which is the one a payment settles. */
  invoiceId: string;

  /** The total of the invoice, as the exact decimal string the contract publishes it as. */
  totalAmountTyiyn: string;

  /** Where the payment of that invoice stands. */
  payment: Payment;
};

/** What the interface shows about one completed ride. */
export type CompletedRideResult = {
  /** The ride the result is about, which is what the record of this tab is matched against. */
  rentalId: string;

  /** The vehicle of the ride, when the answer this tab received named one. */
  vehicle?: string;

  /** Why the ride ended, with the sources that ran out when an empty one ended it. */
  completion: Completion;

  /** The moment the ride ended, which is not the moment the notification about it was written. */
  endedAt: string;

  /** What the ride cost and how it is being paid for, once its invoice has been read. */
  charge?: CompletedRideCharge;
};

/**
 * completedNotification answers the newest notification about a ride that ended, or undefined when
 * the account holds none. The collection is ordered by the moment each notification was written, so
 * the first one of that kind reports the ride the account finished last, which is the ride a panel
 * below an ending is about.
 */
export function completedNotification(
  collection: NotificationCollection | undefined,
): RentalCompletedNotification | undefined {
  return collection?.items.find(isCompletedRide);
}

/**
 * completedResult combines the notification about a ride that ended with the invoice it names. The
 * reason and the moment come from the notification, which is what a reload of any tab finds; the
 * amount and the state of the payment come from the invoice alone, so they are left out until it has
 * been read rather than guessed at.
 */
export function completedResult(
  notification: RentalCompletedNotification | undefined,
  invoice: InvoiceView | undefined,
): CompletedRideResult | undefined {
  if (notification === undefined) return undefined;

  const result: CompletedRideResult = {
    rentalId: notification.rental_id,
    completion: notification.completion,
    endedAt: notification.ended_at,
  };

  const charge = chargeOf(notification.invoice_id, invoice);
  if (charge === undefined) return result;

  return { ...result, charge };
}

/**
 * recordedResult is the result as the record of a ride this tab ended states it. It is the immediate
 * answer for a finish this tab sent, before the notification about that ride has been read, and it
 * carries the two things no other answer the interface reads publishes: the vehicle, and the state of
 * a payment the service has since moved.
 */
export function recordedResult(record: CompletedRide): CompletedRideResult {
  return {
    rentalId: record.finished.rental.id,
    vehicle: record.finished.rental.vehicle.model,
    completion: record.finished.rental.completion,
    endedAt: record.finished.rental.completed_at,
    charge: {
      invoiceId: record.payment.invoice.id,
      totalAmountTyiyn: record.payment.invoice.total_amount_tyiyn,
      payment: record.payment.payment,
    },
  };
}

/**
 * shownResult is the result the panel shows. What the service published wins whenever there is any of
 * it: a record of the ride the service itself reported is a memory of this tab, and the service's own
 * answer is what a person is charged by. The record answers while there is nothing published yet, and
 * it names the vehicle even then, because the completion notification does not carry it.
 */
export function shownResult(
  published: CompletedRideResult | undefined,
  record: CompletedRide | undefined,
): CompletedRideResult | undefined {
  if (published !== undefined) return withNamedVehicle(published, record);
  if (record === undefined) return undefined;

  return recordedResult(record);
}

/**
 * invoiceWorthReading reports whether reading the invoice again can still change what is shown.
 * Nothing read yet is worth reading — the amount and the state of the payment are what the invoice is
 * read for — and a pending payment is the attempt the service owes the invoice, which it makes
 * without this tab doing anything. A state the service has settled changes only when a payment is
 * sent, and the sender of that payment asks for the read itself rather than waiting for one.
 */
export function invoiceWorthReading(payment: Payment | undefined): boolean {
  return payment === undefined || payment.status === 'pending';
}

/** Whether one notification reports a ride that ended, which is the one a result is read from. */
function isCompletedRide(notification: Notification): notification is RentalCompletedNotification {
  return notification.type === 'rental_completed';
}

/**
 * What the invoice states about the ride, or nothing when the invoice read is not this ride's. An
 * invoice the notification does not name is refused rather than shown: the read of the invoice it
 * does name may still be on its way, and the amount of the ride before this one is not this ride's.
 */
function chargeOf(invoiceId: string, invoice: InvoiceView | undefined): CompletedRideCharge | undefined {
  if (invoice === undefined) return undefined;
  if (invoice.invoice.id !== invoiceId) return undefined;

  return { invoiceId, totalAmountTyiyn: invoice.invoice.total_amount_tyiyn, payment: invoice.payment };
}

/** The published result with the vehicle the record names for the same ride, when it names one. */
function withNamedVehicle(published: CompletedRideResult, record: CompletedRide | undefined): CompletedRideResult {
  if (record === undefined) return published;
  if (record.finished.rental.id !== published.rentalId) return published;

  return { ...published, vehicle: record.finished.rental.vehicle.model };
}
