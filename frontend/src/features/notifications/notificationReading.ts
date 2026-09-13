import type { NotificationCollection } from '../../shared/api/notifications.ts';
import type { Resource } from '../../shared/api/Resource.ts';
import { isNewerTimestamp } from '../events/version.ts';

/** One read of the collection, together with the account it was read for. */
export type NotificationReading = { session: string; collection: NotificationCollection };

/** The collection the interface shows, and the moment the answer that carried it arrived. */
export type ShownCollection = { collection: NotificationCollection; receivedAt: Date };

/**
 * Whether one answer is worth storing over the one already held. An answer computed at a moment not
 * later than the one already held is refused, so a late answer from an older read cannot put a newer
 * version of a notification back on screen: a warning that was read stays read, and one the server
 * has deactivated stays out.
 */
export function updatesCollection(held: NotificationCollection | undefined, incoming: NotificationCollection): boolean {
  if (held === undefined) return true;
  return isNewerTimestamp(incoming.server_time, held.server_time);
}

/**
 * The collection the account on screen may be shown, or nothing. The answer carries the account it
 * was read for, so a switch to another one shows nothing of the previous account — whether the read
 * of the new one is still on its way, failed, or never happens at all.
 */
export function shownCollection(
  resource: Resource<NotificationReading | undefined>,
  session: string | undefined,
): ShownCollection | undefined {
  if (resource.phase !== 'ready' && resource.phase !== 'stale') return undefined;

  const reading = resource.value;
  if (reading === undefined || session === undefined || reading.session !== session) return undefined;

  return { collection: reading.collection, receivedAt: resource.loadedAt };
}
