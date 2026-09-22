import { useCallback, useMemo, useState } from 'react';
import { fetchNotifications, markNotificationRead } from '../../shared/api/notifications.ts';
import { identityOf } from '../../shared/account/identity.ts';
import type { NotificationReading, Notifications, ReadPhase } from '../../shared/account/notifications.ts';
import type { Account } from '../../shared/account/session.ts';
import { createReadCoordinator, type ReadCoordinator } from '../../shared/read/coordinator.ts';
import type { DocumentKind } from '../../shared/read/readCycle.ts';
import { useReadCycle } from '../../shared/read/useReadCycle.ts';
import type { CycleFeed } from '../../shared/read/useReadCycle.ts';
import { useResource, type ResourceRead } from '../../shared/read/useResource.ts';
import { notificationAnswers } from './notificationAnswers.ts';
import { shownCollection } from './notificationReading.ts';

/** The one document the reader of the account's own notifications keeps up to date. */
const COLLECTION_DOCUMENTS: readonly DocumentKind[] = ['notifications'];

export type { Notifications } from '../../shared/account/notifications.ts';

/** What one account's mark read was answered with, kept under the account it belongs to. */
type AnsweredRead = { session: string | undefined; phase: ReadPhase };

/**
 * useNotifications reads the notifications addressed to the signed-in person and keeps them up to
 * date from the private stream and from its own reconciliation, which is the one read cycle every
 * reader takes part in: a change signal asks for a read, a handshake asks for one again, and the
 * interval repairs a signal nobody received. The stream is only a signal — what a warning says is
 * the collection the server answers with.
 *
 * An account change builds a new reader, and the answer itself carries the account it was read for,
 * so the notifications of the previous one leave the screen even when the read of the next one never
 * arrives. What the browser remembers about marking one read is keyed by the account in the same way,
 * so the next account starts with nothing to report about a command of the previous one. Signed out,
 * the reader reads nothing at all and no request is made.
 */
export function useNotifications(account: Account, events: CycleFeed): Notifications {
  const { session, csrfToken } = identityOf(account);

  const coordinator = useMemo(() => coordinatorFor(session), [session]);
  const read = useMemo(
    () => (coordinator === undefined || session === undefined ? undefined : readOf(coordinator, session)),
    [coordinator, session],
  );

  const load = useCallback((signal: AbortSignal) => readCollection(session, signal), [session]);
  const handle = useResource<NotificationReading | undefined>(load, read);

  useReadCycle(coordinator, events, COLLECTION_DOCUMENTS);

  const [answered, setAnswered] = useState<AnsweredRead>(() => ({ session, phase: { state: 'idle' } }));
  const phase = answered.session === session ? answered.phase : { state: 'idle' as const };

  const markRead = useCallback(
    (notificationId: string) => {
      if (csrfToken === undefined || coordinator === undefined) return;

      setAnswered({ session, phase: { state: 'sending' } });
      void markNotificationRead(notificationId, csrfToken).then((marked) => {
        // Whatever the answer was, the collection is read again: a read whose answer was lost may
        // still have been stored, and what the warning shows is what the server holds.
        coordinator.force('notifications');
        setAnswered({ session, phase: marked ? { state: 'idle' } : { state: 'failed' } });
      });
    },
    [csrfToken, coordinator, session],
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
