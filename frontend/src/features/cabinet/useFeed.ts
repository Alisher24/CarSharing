import { useCallback, useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react';
import { afterFailure, loadedValue, type Resource } from '../../shared/api/Resource.ts';
import { storedAnswer } from '../events/answerStore.ts';
import { createReadCoordinator, type ReadCoordinator } from '../events/coordinator.ts';
import { usePrivateCycle } from '../events/privateCycle.ts';
import type { DocumentKind } from '../events/readCycle.ts';
import type { PrivateFeed } from '../events/usePrivateEvents.ts';
import { useRequestedReads } from '../events/useRequestedReads.ts';
import { feedAnswers, feedSession } from './feedAnswers.ts';
import {
  continuationCursor,
  type FeedPage,
  type FeedReading,
  type FeedRecord,
  type Placement,
  type Replaces,
} from './feedPages.ts';

/**
 * Reads one page of a feed: the newest page when no cursor is given, the page after the cursor when
 * one is. It is declared beside the feed rather than rebuilt on every render, because a new one
 * would start the reading again.
 */
export type ReadFeedPage<T extends FeedRecord> = (
  cursor: string | undefined,
  signal: AbortSignal,
) => Promise<{ records: readonly T[]; nextCursor: string | null }>;

/** How one feed reads itself. Both functions are declared beside the feed rather than per render. */
export type FeedOptions<T extends FeedRecord> = {
  /** The account the feed belongs to, or nothing when nobody is signed in. */
  session: string | undefined;

  /** The document a private change signal names these records by. */
  document: DocumentKind;

  /** The private stream the feed is kept fresh from. */
  events: PrivateFeed;

  read: ReadFeedPage<T>;
  replaces: Replaces<T>;
};

/** What a screen reads about one feed, and the two things a person can ask it for. */
export type Feed<T extends FeedRecord> = {
  /** The records on screen and how the last read of them went. */
  resource: Resource<FeedReading<T>>;

  /** Whether a page after the records on screen can still be read. */
  continues: boolean;

  /** Reads the page after the records on screen and adds it below them. */
  readMore: () => void;

  /** Reads what the last attempt asked for again, which is what a person asks after a failure. */
  retry: () => void;
};

/** One read of a feed: which page it asks for, and where that page starts. */
type FeedRequest = { placement: Placement; cursor: string | undefined };

/** The request every feed starts from and comes back to, which is the newest page of it. */
const NEWEST_PAGE: FeedRequest = { placement: 'newest', cursor: undefined };

/**
 * useFeed keeps one paginated feed of the signed-in person up to date. It takes the same part in the
 * read cycle as every other private reader: a change signal, a handshake and the reconciliation ask
 * for the newest page, and a person asks for the page after the one they have reached.
 *
 * Reading again is a merge rather than a replacement, which is what makes a feed a person is
 * scrolling through survive a signal: what is on screen stays, a record that moved is replaced where
 * it stands, and the newest records join above it.
 *
 * An account change builds a new reader, so nothing read for the previous account can be shown to
 * the next one. Signed out, the feed reads nothing at all.
 */
export function useFeed<T extends FeedRecord>(options: FeedOptions<T>): Feed<T> {
  const { session, document, events, read, replaces } = options;

  const documents = useMemo(() => [document], [document]);
  const coordinator = useMemo(() => coordinatorFor<T>(session, document, replaces), [session, document, replaces]);
  usePrivateCycle(coordinator, events, documents);

  const [resource, setResource] = useState<Resource<FeedReading<T>>>({ phase: 'loading' });
  const [request, setRequest] = useState<FeedRequest>(NEWEST_PAGE);

  useAccountReset(session, setResource, setRequest);
  useNewestPageWhenAsked(coordinator, document, setRequest);
  useReadOf({ coordinator, document, session, read, request, setResource });

  const continuation = loadedValue(resource)?.continuation;
  const readMore = useCallback(() => {
    const cursor = continuation === undefined ? undefined : continuationCursor(continuation);
    if (cursor !== undefined) setRequest({ placement: 'continuation', cursor });
  }, [continuation]);

  const retry = useCallback(() => setRequest((asked) => ({ ...asked })), []);

  return { resource, continues: continuation?.state === 'more', readMore, retry };
}

/**
 * The account on screen changed: the reader is a new one, and nothing read for the previous account
 * may stay on the screen the next one is shown.
 */
function useAccountReset<T extends FeedRecord>(
  session: string | undefined,
  setResource: Dispatch<SetStateAction<Resource<FeedReading<T>>>>,
  setRequest: Dispatch<SetStateAction<FeedRequest>>,
): void {
  useEffect(() => {
    setResource({ phase: 'loading' });
    setRequest(NEWEST_PAGE);
  }, [session, setResource, setRequest]);
}

/**
 * A change signal, a handshake and the reconciliation all ask for the newest page: what a re-read is
 * for is the records the account has gained, and the pages below the ones on screen are still
 * reached by the cursor a person already continued from.
 */
function useNewestPageWhenAsked(
  coordinator: ReadCoordinator | undefined,
  document: DocumentKind,
  setRequest: Dispatch<SetStateAction<FeedRequest>>,
): void {
  const requested = useRequestedReads(coordinator, document);

  useEffect(() => {
    if (requested > 0) setRequest({ ...NEWEST_PAGE });
  }, [requested, setRequest]);
}

/** Everything one read of a feed is performed with. */
type FeedRead<T extends FeedRecord> = {
  coordinator: ReadCoordinator | undefined;
  document: DocumentKind;
  session: string | undefined;
  read: ReadFeedPage<T>;
  request: FeedRequest;
  setResource: Dispatch<SetStateAction<Resource<FeedReading<T>>>>;
};

/**
 * useReadOf performs the read the request names, once, and stores what it answered. A read still in
 * flight when the next request arrives is abandoned, so its answer can neither be stored nor compete
 * with the answer to the request that replaced it.
 */
function useReadOf<T extends FeedRecord>(options: FeedRead<T>): void {
  const { coordinator, document, session, read, request, setResource } = options;

  useEffect(() => {
    if (coordinator === undefined || session === undefined) return;

    const controller = new AbortController();
    let abandoned = false;

    async function reload(): Promise<void> {
      const ticket = coordinator?.begin(document);
      if (coordinator === undefined || ticket === undefined) return;

      try {
        const page = await read(request.cursor, controller.signal);
        if (abandoned) return;

        const answered: FeedPage<T> = { ...page, placement: request.placement };
        const stored = storedAnswer<FeedReading<T>>(coordinator, ticket, session, answered);
        if (stored !== undefined) setResource(stored);
      } catch {
        // A failed read keeps the records already on screen and marks them stale, so a feed a person
        // is reading does not empty itself because one attempt did not arrive.
        if (!abandoned) setResource(afterFailure);
      } finally {
        if (!abandoned) coordinator.settle(document);
      }
    }

    void reload();

    return () => {
      abandoned = true;
      controller.abort();
      coordinator.settle(document);
    };
  }, [coordinator, document, session, read, request, setResource]);
}

/** The reader of one session, which carries the feed only that session may be shown. */
function coordinatorFor<T extends FeedRecord>(
  session: string | undefined,
  document: DocumentKind,
  replaces: Replaces<T>,
): ReadCoordinator | undefined {
  if (session === undefined) return undefined;

  return createReadCoordinator(session, feedAnswers(document, feedSession<T>(session), replaces));
}
