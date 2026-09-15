import { fetchRides, type RideSummary } from '../../shared/api/history.ts';
import { sessionOf, type Account } from '../account/useAccount.ts';
import type { DocumentKind } from '../events/readCycle.ts';
import type { PrivateFeed } from '../events/usePrivateEvents.ts';
import type { Replaces } from './feedPages.ts';
import { useFeed, type Feed, type ReadFeedPage } from './useFeed.ts';

/**
 * The document a change to one of the account's rentals is signalled under. The history grows only
 * when a ride ends, which is one of the changes that signal announces, so the feed is read again on
 * the same signal the panel of the rental in force is read on.
 */
const RIDES_DOCUMENT: DocumentKind = 'rentals';

/** A ride that has ended never changes, so the reading on screen is the reading of it. */
const RIDE_NEVER_CHANGES: Replaces<RideSummary> = () => false;

const readRidePage: ReadFeedPage<RideSummary> = (cursor, signal) =>
  fetchRides(cursor, signal).then((page) => ({ records: page.items, nextCursor: page.next_cursor }));

/** useRideFeed reads the rides the signed-in person has finished, newest first. */
export function useRideFeed(account: Account, events: PrivateFeed): Feed<RideSummary> {
  return useFeed<RideSummary>({
    session: sessionOf(account),
    document: RIDES_DOCUMENT,
    events,
    read: readRidePage,
    replaces: RIDE_NEVER_CHANGES,
  });
}
