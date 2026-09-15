import type { Completion, FinishResult, InvoiceView, Payment } from '../../shared/api/current.ts';
import type {
  Notification,
  NotificationCollection,
  RentalCompletedNotification,
} from '../../shared/api/notifications.ts';
/**
 * What a person is shown about a ride that ended: why it ended, the moment it actually ended, what it
 * cost and where paying for it stands.
 *
 * The result is read from the account's own notifications and from the invoice the completion names,
 * so it is the same on the screen that ended the ride, after a reload and in another tab. The answer
 * to a finish this tab sent is only what the result is shown from in the moment before the service's
 * own report of that ending has been read, and the vehicle is named by that answer alone: the feed of
 * finished rides is where a vehicle is named once the panel is no longer showing the ride.
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

  /** The vehicle of the ride, when the answer to a finish this tab sent named one. */
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
 * finishedResult is the result as the answer to a finish this tab sent states it. It is the whole
 * truth about that ending — the ride, the vehicle, and the invoice as it was issued — and it answers
 * for the moment between that answer and the service's own report of the same ending.
 */
export function finishedResult(finished: FinishResult): CompletedRideResult {
  return {
    rentalId: finished.rental.id,
    vehicle: finished.rental.vehicle.model,
    completion: finished.rental.completion,
    endedAt: finished.rental.completed_at,
    charge: {
      invoiceId: finished.invoice.invoice.id,
      totalAmountTyiyn: finished.invoice.invoice.total_amount_tyiyn,
      payment: finished.invoice.payment,
    },
  };
}

/**
 * shownResult is the result the panel shows. What the service published wins whenever there is any of
 * it: the answer to a finish is a memory of this tab, and the service's own answer is what a person
 * is charged by. The answer to the finish shows while there is nothing published yet, and it names
 * the vehicle even afterwards, because the report of a completed ride does not carry one.
 */
export function shownResult(
  published: CompletedRideResult | undefined,
  finished: FinishResult | undefined,
): CompletedRideResult | undefined {
  if (published !== undefined) return withNamedVehicle(published, finished);
  if (finished === undefined) return undefined;

  return finishedResult(finished);
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

/** The published result with the vehicle a finish this tab sent named, when it named the same ride. */
function withNamedVehicle(published: CompletedRideResult, finished: FinishResult | undefined): CompletedRideResult {
  if (finished === undefined) return published;
  if (finished.rental.id !== published.rentalId) return published;

  return { ...published, vehicle: finished.rental.vehicle.model };
}
