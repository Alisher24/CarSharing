import { useCallback, useMemo } from 'react';
import type { CurrentSnapshot } from '../../shared/api/current.ts';
import { fetchCurrentRental } from '../../shared/api/current.ts';
import { identityOf } from '../../shared/account/identity.ts';
import type { Account } from '../../shared/account/session.ts';
import { createReadCoordinator, type ReadCoordinator } from '../../shared/read/coordinator.ts';
import type { Resource } from '../../shared/read/Resource.ts';
import type { DocumentKind } from '../../shared/read/readCycle.ts';
import { useReadCycle } from '../../shared/read/useReadCycle.ts';
import { useResource, type ResourceRead } from '../../shared/read/useResource.ts';
import type { CycleFeed } from '../../shared/read/useReadCycle.ts';
import { currentAnswers } from './currentAnswers.ts';

/** The one document the reader of the account's own reservation keeps up to date. */
const CURRENT_DOCUMENTS: readonly DocumentKind[] = ['rentals'];

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
 * stream and from its own reconciliation, which is the one read cycle every reader takes part in. The
 * resources a person reads are read by separate coordinators — the public catalog belongs to no
 * session, and the reservation may only ever belong to one account — and each private reader states
 * the documents it holds.
 *
 * An account change builds a new reader: nothing observed, covered or answered before it can decide
 * what the next account is shown. Signed out, the reader reads nothing at all and no request is made.
 */
export function useCurrentRental(account: Account, events: CycleFeed): CurrentRental {
  const session = identityOf(account).session;

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

  useReadCycle(coordinator, events, CURRENT_DOCUMENTS);

  return { resource: handle.resource, retry: handle.retry, session };
}

/** The reader of one session, which carries the last answer only that session may compare against. */
function coordinatorFor(session: string | undefined): ReadCoordinator | undefined {
  if (session === undefined) return undefined;

  return createReadCoordinator(session, currentAnswers({ session, reading: { current: undefined } }));
}

function readOf(coordinator: ReadCoordinator, session: string): ResourceRead {
  return { coordinator, document: 'rentals', session };
}
