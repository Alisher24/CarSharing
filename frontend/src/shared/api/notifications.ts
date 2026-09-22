import { getNotifications, readNotification } from './generated/sdk.gen';
import { originHeader, sameOriginRequest } from './request.ts';
import type { NotificationCollection } from './generated/types.gen';

export type {
  Notification,
  NotificationCollection,
  RentalCompletedNotification,
  ReservationExpiringNotification,
} from './generated/types.gen';

/**
 * fetchNotifications reads the newest page of the caller's own notifications. The page is the window
 * the warning is read from: a reservation is made before the warning about it, and marking a
 * notification read moves its version rather than its position in the collection, so the warning of
 * the reservation in force is the newest notification an account holds. The history behind that page
 * belongs to the cabinet, which asks for the pages a cursor continues.
 */
export async function fetchNotifications(signal: AbortSignal): Promise<NotificationCollection> {
  const { data } = await getNotifications({ ...sameOriginRequest, throwOnError: true, signal });
  return data;
}

/**
 * markNotificationRead marks one notification read, and reports whether the server answered that it
 * stored the read. The operation is idempotent, so a repeat answers the same final notification; a
 * transport failure, a refusal and a session that has ended are all reported as "not answered",
 * because the caller settles the question by reading the collection rather than by reasoning about
 * an answer it never received.
 */
export async function markNotificationRead(notificationId: string, csrfToken: string): Promise<boolean> {
  try {
    const { data } = await readNotification({
      path: { id: notificationId },
      headers: { ...originHeader(), 'X-CSRF-Token': csrfToken },
      ...sameOriginRequest,
    });
    return data !== undefined;
  } catch {
    return false;
  }
}
