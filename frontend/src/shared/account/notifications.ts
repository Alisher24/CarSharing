import type { NotificationCollection } from '../api/notifications.ts';
import type { Resource } from '../read/Resource.ts';

/**
 * The state of the notifications addressed to the signed-in person: the collection as the account's
 * own reader holds it, together with the account it was read for.
 *
 * It is declared above both the feature that reads the collection and the two features that show it,
 * because a warning about a reservation and the result of a finished ride are read from the same
 * answer rather than from two readings of it.
 */
export type NotificationReading = { session: string; collection: NotificationCollection };

/** The collection one account holds, once the reading of it belongs to the account on screen. */
export type ShownCollection = { collection: NotificationCollection; receivedAt: Date };

/** Where the last «Прочитано» stands. */
export type ReadPhase = { state: 'idle' } | { state: 'sending' } | { state: 'failed' };

/** What the interface reads about the notifications addressed to the signed-in person. */
export type Notifications = {
  /** The collection read for the account on screen; nothing read for another one is ever shown. */
  reading: ShownCollection | undefined;

  /** Where the last attempt to mark a warning read stands. */
  read: ReadPhase;

  /** Marks one notification read on the server, which is what the warning's one action does. */
  markRead: (notificationId: string) => void;

  /** The session these answers belong to, or undefined when nobody is signed in. */
  session: string | undefined;
};

/** One reading of the collection together with the resource it was read into. */
export type ReadingResource = Resource<NotificationReading | undefined>;
