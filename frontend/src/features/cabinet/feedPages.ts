/**
 * How the pages of one feed become the list a person reads. A feed is read again whenever the
 * service says the account's records moved, and reading again is a merge rather than a replacement:
 * what is on screen stays there, a record that came back changed is replaced where it stands, and
 * what is new joins it.
 *
 * Replacing the newest page instead would lose a record that crossed the page boundary: the pages
 * below it were handed out by a cursor taken from the record that used to end the newest page, and a
 * record pushed off that page appears neither in the new newest page nor behind that cursor. The
 * price of the merge is stated rather than hidden: the list may hold a record that a fresh read
 * would have put on another page.
 */

/** A record of a feed is addressed by its identifier, which is what a merge matches records by. */
export type FeedRecord = { id: string };

/** Where a page's records belong among the ones already held. */
export type Placement = 'newest' | 'continuation';

/** One page as the service answered it, and what it was asked for. */
export type FeedPage<T extends FeedRecord> = {
  records: readonly T[];
  nextCursor: string | null;
  placement: Placement;
};

/**
 * Whether a record that came back replaces the one already held. A record that never changes answers
 * no to every reading of itself; one that does is replaced only by a reading that is newer than the
 * one on screen, so an answer that overtook another cannot put an older state back.
 */
export type Replaces<T extends FeedRecord> = (held: T, answered: T) => boolean;

/** Where the page after the ones on screen starts. */
export type Continuation = { state: 'start' } | { state: 'more'; cursor: string } | { state: 'end' };

/** The records on screen and where the page after them starts. */
export type FeedReading<T extends FeedRecord> = {
  records: readonly T[];
  continuation: Continuation;
};

/** emptyReading is a feed nothing has been read into yet, which is what a reader starts from. */
export function emptyReading<T extends FeedRecord>(): FeedReading<T> {
  return { records: [], continuation: { state: 'start' } };
}

/** mergedReading folds one page into the feed on screen. */
export function mergedReading<T extends FeedRecord>(
  held: FeedReading<T>,
  page: FeedPage<T>,
  replaces: Replaces<T>,
): FeedReading<T> {
  return {
    records: mergedRecords(held.records, page, replaces),
    continuation: continuedAfter(held.continuation, page),
  };
}

/**
 * mergedRecords folds one page into the records on screen. A record the page names again is replaced
 * where it stands, so nothing moves under a person reading it; a record the page names for the first
 * time joins the list where that page belongs — above everything for the newest page, below
 * everything for a continuation.
 */
function mergedRecords<T extends FeedRecord>(
  held: readonly T[],
  page: FeedPage<T>,
  replaces: Replaces<T>,
): readonly T[] {
  const answered = new Map(page.records.map((record) => [record.id, record]));
  const kept = held.map((record) => replacementOf(record, answered.get(record.id), replaces));

  const shown = new Set(held.map((record) => record.id));
  const fresh = page.records.filter((record) => !shown.has(record.id));
  if (fresh.length === 0) return kept;

  return page.placement === 'newest' ? [...fresh, ...kept] : [...kept, ...fresh];
}

/** The record to keep on screen: the one that came back, when it is the newer of the two. */
function replacementOf<T extends FeedRecord>(held: T, answered: T | undefined, replaces: Replaces<T>): T {
  if (answered === undefined) return held;

  return replaces(held, answered) ? answered : held;
}

/**
 * continuedAfter is where the page after the feed starts once one page has been read. Only a read
 * that extended the feed moves it: re-reading the newest page says nothing about where the records
 * below the ones on screen continue, and taking its cursor would ask for the second page again.
 */
function continuedAfter(held: Continuation, page: FeedPage<FeedRecord>): Continuation {
  const extended = page.placement === 'continuation' || held.state === 'start';
  if (!extended) return held;
  if (page.nextCursor === null) return { state: 'end' };

  return { state: 'more', cursor: page.nextCursor };
}

/** The cursor the next read of a feed continues from, or nothing when it reads the newest page. */
export function continuationCursor(continuation: Continuation): string | undefined {
  return continuation.state === 'more' ? continuation.cursor : undefined;
}
