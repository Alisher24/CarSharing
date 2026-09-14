import type { CurrentSnapshot } from '../../shared/api/current.ts';
import type {
  Notification,
  NotificationCollection,
  ReservationExpiringNotification,
} from '../../shared/api/notifications.ts';
import { currentRental, vehicleName } from '../reservation/reservationCopy.ts';

/** The warning a person sees about the reservation that is running out. */
export type CurrentWarning = {
  /** The notification the one action marks read, which is the identity of the warning itself. */
  notificationId: string;

  /** The vehicle the reservation holds, which is what a person recognises it by. */
  vehicle: string;

  /** The moment the reservation ends, as the collection published it. */
  expiresAt: string;

  /** The moment that answer was computed at, which the remaining time is measured from. */
  serverTime: string;
};

/**
 * currentWarning is the one warning the interface shows, or nothing when there is none to show.
 *
 * A warning is current while the reservation it names is the one in force and is still a reservation:
 * the vehicle is read from that reservation, so a warning about a rental that was cancelled, that
 * expired or that became a ride has nothing to name and is not shown. A notification the server has
 * deactivated, and one that was read, are not current either — read state belongs to the server, so
 * a reload does not warn about the same minute twice.
 */
export function currentWarning(
  collection: NotificationCollection | undefined,
  snapshot: CurrentSnapshot | undefined,
): CurrentWarning | undefined {
  const rental = currentRental(snapshot);
  if (collection === undefined || rental === undefined || rental.state !== 'reserved') return undefined;

  const warning = collection.items.find((one): one is ReservationExpiringNotification =>
    isUnreadWarningOf(one, rental.id),
  );
  if (warning === undefined) return undefined;

  return {
    notificationId: warning.id,
    vehicle: vehicleName(rental),
    expiresAt: warning.expires_at,
    serverTime: collection.server_time,
  };
}

/** Whether one notification is the warning about the stated reservation that nobody has read yet. */
function isUnreadWarningOf(notification: Notification, rentalId: string): boolean {
  if (notification.type !== 'reservation_expiring') return false;

  return notification.rental_id === rentalId && notification.active && notification.read_at === undefined;
}
