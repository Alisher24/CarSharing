import { answersOf } from '../../shared/read/documentAnswers.ts';
import type { AnswerHandlers, DocumentKind } from '../../shared/read/readCycle.ts';
import {
  emptyReading,
  mergedReading,
  type FeedPage,
  type FeedReading,
  type FeedRecord,
  type Replaces,
} from './feedPages.ts';

/** What one session of reading a feed knows, which every answered page is merged into. */
export type FeedSession<T extends FeedRecord> = {
  session: string;
  reading: { feed: FeedReading<T> };
};

/** feedSession is what a reader of one account starts from: an account and an empty feed. */
export function feedSession<T extends FeedRecord>(session: string): FeedSession<T> {
  return { session, reading: { feed: emptyReading<T>() } };
}

/**
 * feedAnswers builds the answers of the coordinator that reads one feed of one account. The feed is
 * the only document it answers for: a signal about any other one belongs to the reader that holds
 * it, whether that reader is the catalog, the panel of the rental in force or the other feed.
 */
export function feedAnswers<T extends FeedRecord>(
  document: DocumentKind,
  state: FeedSession<T>,
  replaces: Replaces<T>,
): Record<DocumentKind, AnswerHandlers> {
  return answersOf({ [document]: pageAnswer(state, replaces) });
}

/**
 * The answer of one page. An answer of another account is refused before it is looked at, and the
 * page that is accepted is merged into the records on screen rather than put in their place.
 *
 * A page covers no version the cycle can stop asking about: a feed publishes a window of the
 * account's records rather than all of them, so a signal about a record below that window would
 * never be covered and the feed reads again on every signal it is told about.
 */
function pageAnswer<T extends FeedRecord>(state: FeedSession<T>, replaces: Replaces<T>): AnswerHandlers {
  return {
    accepts: (session) => session === state.session,
    observe: (value, session) => {
      if (session !== state.session) return undefined;

      state.reading.feed = mergedReading(state.reading.feed, value as FeedPage<T>, replaces);
      return { value: state.reading.feed, covered: new Map() };
    },
  };
}
