import { useCallback, useEffect, useMemo, useState } from 'react';
import { fetchNotifications, markNotificationRead } from '../../shared/api/notifications.ts';
import { useResource, type ResourceRead } from '../../shared/api/useResource.ts';
import { csrfTokenOf, sessionOf, type Account } from '../account/useAccount.ts';
import { createReadCoordinator, type ReadCoordinator } from '../events/coordinator.ts';
import { usePrivateCycle } from '../events/privateCycle.ts';
import type { DocumentKind } from '../events/readCycle.ts';
import type { PrivateFeed } from '../events/usePrivateEvents.ts';
import { notificationAnswers } from './notificationAnswers.ts';
import { shownCollection, type NotificationReading, type ShownCollection } from './notificationReading.ts';

/** The one document the reader of the account's own notifications keeps up to date. */
const COLLECTION_DOCUMENTS: readonly DocumentKind[] = ['notifications'];

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

/**
 * useNotifications reads the notifications addressed to the signed-in person and keeps them up to
 * date from the private stream and from its own reconciliation: a change signal asks for a read, a
 * handshake asks for one again, and the interval repairs a signal nobody received. The stream is only
 * a signal — what a warning says is the collection the server answers with.
 *
 * An account change builds a new reader, and the answer itself carries the account it was read for,
 * so the notifications of the previous one leave the screen even when the read of the next one never
 * arrives. Signed out, the reader reads nothing at all and no request is made.
 */
export function useNotifications(account: Account, events: PrivateFeed): Notifications {
  const session = sessionOf(account);
  const csrfToken = csrfTokenOf(account);

  const coordinator = useMemo(() => coordinatorFor(session), [session]);
  const read = useMemo(
    () => (coordinator === undefined || session === undefined ? undefined : readOf(coordinator, session)),
    [coordinator, session],
  );

  const load = useCallback((signal: AbortSignal) => readCollection(session, signal), [session]);
  const handle = useResource<NotificationReading | undefined>(load, read);

  usePrivateCycle(coordinator, events, COLLECTION_DOCUMENTS);

  const [phase, setPhase] = useState<ReadPhase>({ state: 'idle' });

  // What the browser remembers about a mark read belongs to one account: the next one starts with
  // nothing to report about a command of the previous one.
  useEffect(() => setPhase({ state: 'idle' }), [session]);

  const markRead = useCallback(
    (notificationId: string) => {
      if (csrfToken === undefined || coordinator === undefined) return;

      setPhase({ state: 'sending' });
      void markNotificationRead(notificationId, csrfToken).then((marked) => {
        // Whatever the answer was, the collection is read again: a read whose answer was lost may
        // still have been stored, and what the warning shows is what the server holds.
        coordinator.force('notifications');
        setPhase(marked ? { state: 'idle' } : { state: 'failed' });
      });
    },
    [csrfToken, coordinator],
  );

  return { reading: shownCollection(handle.resource, session), read: phase, markRead, session };
}

/** The reader of one session, which carries the last answer only that session may compare against. */
function coordinatorFor(session: string | undefined): ReadCoordinator | undefined {
  if (session === undefined) return undefined;
  return createReadCoordinator(session, notificationAnswers({ session, reading: { collection: undefined } }));
}

function readOf(coordinator: ReadCoordinator, session: string): ResourceRead {
  return { coordinator, document: 'notifications', session };
}

/**
 * readCollection reads the collection of one account, and answers with the account it was read for.
 * A reader with no session asks for nothing rather than for an answer it would be refused.
 */
function readCollection(session: string | undefined, signal: AbortSignal): Promise<NotificationReading | undefined> {
  if (session === undefined) return Promise.resolve(undefined);
  return fetchNotifications(signal).then((collection) => ({ session, collection }));
}
