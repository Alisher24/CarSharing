import type { ReactNode } from 'react';
import type { AbsenceCopy } from '../../shared/components/absence.ts';
import { ResourceNotice } from '../../shared/components/ResourceNotice.tsx';
import { loadedValue } from '../../shared/read/Resource.ts';
import { found } from '../../shared/read/presence.ts';
import { READ_MORE_ACTION } from './cabinetCopy.ts';
import type { FeedRecord } from './feedPages.ts';
import type { Feed } from './useFeed.ts';

type FeedListProps<T extends FeedRecord> = {
  feed: Feed<T>;
  copy: AbsenceCopy;

  /** What one record is shown as: the only part a feed of rides and one of invoices differ in. */
  row: (record: T) => ReactNode;
};

/**
 * FeedList is what both feeds of the cabinet are made of: the records on screen, the control that
 * reads the page after them, and the notice that explains a screen with nothing on it.
 *
 * An empty feed and a feed that could not be read are two different things to say, so they are two
 * different notices; and a read that failed leaves whatever was already on screen there, because
 * losing the page a person was reading is not what a failed request should cost them.
 */
export function FeedList<T extends FeedRecord>({ feed, copy, row }: FeedListProps<T>) {
  const records = loadedValue(feed.resource)?.records ?? [];

  return (
    <div className="feed">
      {records.length > 0 && (
        <ul className="feed-rows">
          {records.map((record) => (
            <li className="feed-row" key={record.id}>
              {row(record)}
            </li>
          ))}
        </ul>
      )}

      <ResourceNotice found={found(feed.resource, shown(records))} copy={copy} onRetry={feed.retry} />

      {feed.continues && (
        <button className="action-button" type="button" onClick={feed.readMore}>
          {READ_MORE_ACTION}
        </button>
      )}
    </div>
  );
}

/**
 * The records the notice is answered about: what is on screen, or nothing at all. A notice explains
 * an empty screen, so having nothing is the whole of what it needs to be told.
 */
function shown<T extends FeedRecord>(records: readonly T[]): readonly T[] | undefined {
  return records.length > 0 ? records : undefined;
}
