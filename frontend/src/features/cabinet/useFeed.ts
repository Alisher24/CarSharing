import { useCallback, useEffect, useMemo, useState } from 'react';
import type { Resource } from '../../shared/read/Resource.ts';
import { loadedValue } from '../../shared/read/Resource.ts';
import { createReadCoordinator, type ReadCoordinator } from '../../shared/read/coordinator.ts';
import type { DocumentKind } from '../../shared/read/readCycle.ts';
import { useReadCycle } from '../../shared/read/useReadCycle.ts';
import type { CycleFeed } from '../../shared/read/useReadCycle.ts';
import { useReadProtocol, type LoadDocument } from '../../shared/read/useReadProtocol.ts';
import { useRequestedReads } from '../../shared/read/useRequestedReads.ts';
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
  events: CycleFeed;

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

/** What one account's feed holds: the page the last attempt asked for, and what it read for. */
type HeldFeed = {
  session: string | undefined;
  request: FeedRequest;
  replay: number;
};

/** What a feed of one account starts from: the newest page asked for, and nothing asked for yet. */
function heldFor(session: string | undefined): HeldFeed {
  return { session, request: NEWEST_PAGE, replay: 0 };
}

/**
 * useFeed keeps one paginated feed of the signed-in person up to date. It takes the same part in the
 * read cycle as every other private reader: a change signal, a handshake and the reconciliation ask
 * for the newest page, and a person asks for the page after the one they have reached. The page
 * itself is read through `useReadProtocol`, which is the one way this application reads a document:
 * the feed states what to read and how to merge the answer, and owns nothing else about reading.
 *
 * Reading again is a merge rather than a replacement, which is what makes a feed a person is
 * scrolling through survive a signal: what is on screen stays, a record that moved is replaced where
 * it stands, and the newest records join above it.
 *
 * What was read belongs to the account it was read for, and it is keyed by the account rather than
 * cleared by an effect: the next account starts with nothing of the previous one on screen, from the
 * first render it is shown in. Signed out, the feed reads nothing at all.
 */
export function useFeed<T extends FeedRecord>(options: FeedOptions<T>): Feed<T> {
  const { session, document, events, read, replaces } = options;

  const documents = useMemo(() => [document], [document]);
  const coordinator = useMemo(() => coordinatorFor<T>(session, document, replaces), [session, document, replaces]);
  const [held, setHeld] = useState<HeldFeed>(() => heldFor(session));

  const current = held.session === session ? held : heldFor(session);
  const { request, replay } = current;

  const load = useCallback<LoadDocument<FeedReading<T>>>((signal) => readPage(read, request, signal), [read, request]);
  const reading = useReadProtocol<FeedReading<T>>(load, { coordinator, document, session }, { request, replay });

  useReadCycle(coordinator, events, documents);
  useNewestPageWhenAsked(coordinator, document, setHeld);

  const continuation = loadedValue(reading.resource)?.continuation;
  const readMore = useCallback(() => {
    const cursor = continuation === undefined ? undefined : continuationCursor(continuation);
    if (cursor !== undefined) setHeld((asked) => ({ ...asked, request: { placement: 'continuation', cursor } }));
  }, [continuation]);

  return {
    resource: reading.resource,
    continues: continuation?.state === 'more',
    readMore,
    retry: reading.repeat,
  };
}

/**
 * One page as the service answered it, marked with what it was asked for: a page that continues the
 * feed joins below the records on screen, and the newest page joins above them.
 */
async function readPage<T extends FeedRecord>(
  read: ReadFeedPage<T>,
  request: FeedRequest,
  signal: AbortSignal,
): Promise<FeedReading<T>> {
  const page: FeedPage<T> = { ...(await read(request.cursor, signal)), placement: request.placement };

  return { records: page.records, continuation: continuationOf(page) };
}

/** Where the page after the feed starts once this page has been read. */
function continuationOf<T extends FeedRecord>(page: FeedPage<T>): FeedReading<T>['continuation'] {
  if (page.nextCursor === null) return { state: 'end' };

  return { state: 'more', cursor: page.nextCursor };
}

/**
 * A change signal, a handshake and the reconciliation all ask for the newest page: what a re-read is
 * for is the records the account has gained, and the pages below the ones on screen are still
 * reached by the cursor a person already continued from.
 */
function useNewestPageWhenAsked(
  coordinator: ReadCoordinator | undefined,
  document: DocumentKind,
  setHeld: (update: (held: HeldFeed) => HeldFeed) => void,
): void {
  const requested = useRequestedReads(coordinator, document);

  useEffect(() => {
    if (requested > 0) setHeld((held) => ({ ...held, request: NEWEST_PAGE, replay: held.replay + 1 }));
  }, [requested, setHeld]);
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
