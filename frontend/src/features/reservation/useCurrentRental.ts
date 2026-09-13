import { useCallback, useEffect, useMemo } from 'react';
import type { CurrentSnapshot } from '../../shared/api/current.ts';
import type { Resource } from '../../shared/api/Resource.ts';
import { fetchCurrentRental } from '../../shared/api/current.ts';
import { useResource, type ResourceRead } from '../../shared/api/useResource.ts';
import type { Account } from '../account/useAccount.ts';
import { changedResources } from '../events/changes.ts';
import { createReadCoordinator, type ReadCoordinator } from '../events/coordinator.ts';
import type { PrivateFeed } from '../events/usePrivateEvents.ts';
import { RECONCILE_MILLISECONDS, reconciliationEnabled } from '../events/reconciliation.ts';
import type { DocumentKind } from '../events/readCycle.ts';
import { currentAnswers } from './currentAnswers.ts';

/**
 * How long a batch of private changes is collected before it is read, which is the window the
 * catalog uses for its own: a burst of changes is one read rather than one per change.
 */
const COALESCE_MILLISECONDS = 100;

/** The one document the reader of the account's own reservation keeps up to date. */
const CURRENT_DOCUMENTS: readonly DocumentKind[] = ['current'];

/** What the interface reads about the signed-in person's own reservation. */
export type CurrentRental = {
  /** The last answer about the reservation, and how it is doing. */
  resource: Resource<CurrentSnapshot | undefined>;

  /** Reads the reservation again, which is what a command asks for once it is answered. */
  retry: () => void;

  /** The session these answers belong to, or undefined when nobody is signed in. */
  session: string | undefined;
};

/**
 * useCurrentRental reads the account's own reservation and keeps it up to date from the private
 * stream and from its own reconciliation. The two resources a person reads — the public catalog and
 * their own reservation — are read by separate coordinators, because one belongs to no session and
 * the other may only ever belong to one account.
 *
 * An account change builds a new reader: nothing observed, covered or answered before it can decide
 * what the next account is shown. Signed out, the reader reads nothing at all and no request is made.
 */
export function useCurrentRental(account: Account, events: PrivateFeed): CurrentRental {
  const session = account.state === 'signed-in' ? account.snapshot.user.id : undefined;

  const coordinator = useMemo(() => coordinatorFor(session), [session]);
  const read = useMemo(
    () => (coordinator === undefined || session === undefined ? undefined : readOf(coordinator, session)),
    [coordinator, session],
  );

  // A reader with no session asks for nothing rather than for an answer it would be refused: the
  // private resource does not exist without an account.
  const load = useCallback(
    (signal: AbortSignal) => (session === undefined ? Promise.resolve(undefined) : fetchCurrentRental(signal)),
    [session],
  );
  const handle = useResource<CurrentSnapshot | undefined>(load, read);

  usePrivateChanges(coordinator, events.changes);
  useReadyFrame(coordinator, events.readyCount);
  usePrivateReconciliation(coordinator);

  return { resource: handle.resource, retry: handle.retry, session };
}

/** The reader of one session, which carries the last answer only that session may compare against. */
function coordinatorFor(session: string | undefined): ReadCoordinator | undefined {
  if (session === undefined) return undefined;
  return createReadCoordinator(session, currentAnswers({ session, reading: { current: undefined } }));
}

function readOf(coordinator: ReadCoordinator, session: string): ResourceRead {
  return { coordinator, document: 'current', session };
}

/**
 * Feeds the changes one private connection delivered into the cycle. Each change is remembered at
 * once, so one that arrives during a request is not lost, and the reads they ask for are opened
 * together once the window has passed.
 */
function usePrivateChanges(coordinator: ReadCoordinator | undefined, changes: PrivateFeed['changes']): void {
  useEffect(() => {
    if (coordinator === undefined) return;

    for (const change of changes) coordinator.observe(change.resource, change.id, change.version);

    const batch = window.setTimeout(() => askFor(coordinator, changedResources(changes)), COALESCE_MILLISECONDS);
    return () => window.clearTimeout(batch);
  }, [coordinator, changes]);
}

/** Asks for a read of each private resource a window named. */
function askFor(coordinator: ReadCoordinator, documents: readonly DocumentKind[]): void {
  for (const document of documents) {
    if (coordinator.due(document)) coordinator.force(document);
  }
}

/** Answers a handshake by asking for the reservation to be read again, reconnects included. */
function useReadyFrame(coordinator: ReadCoordinator | undefined, readyCount: number): void {
  useEffect(() => {
    if (coordinator !== undefined && readyCount > 0) coordinator.request(CURRENT_DOCUMENTS);
  }, [coordinator, readyCount]);
}

/**
 * Asks for the reservation to be read again on the same interval the catalog uses. This is what
 * repairs a signal that never arrived, a stream that was down while the change happened, and the
 * day's allowance being restored at local midnight, which no signal announces.
 */
function usePrivateReconciliation(coordinator: ReadCoordinator | undefined): void {
  useEffect(() => {
    if (coordinator === undefined) return;

    const repeat = window.setInterval(() => {
      if (reconciliationEnabled()) coordinator.request(CURRENT_DOCUMENTS);
    }, RECONCILE_MILLISECONDS);
    return () => window.clearInterval(repeat);
  }, [coordinator]);
}
