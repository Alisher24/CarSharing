import { Link } from 'react-router';
import { invoiceAddress } from '../../app/addresses.ts';
import type { Account } from '../../shared/account/session.ts';
import type { CycleFeed } from '../../shared/read/useReadCycle.ts';
import { OPEN_INVOICE } from '../../shared/copy.ts';
import { RIDES_ABSENCE } from './cabinetCopy.ts';
import { FeedList } from './FeedList.tsx';
import { FeedValue } from './FeedValue.tsx';
import { RIDE_COMPLETED_AT, RIDE_REASON, RIDE_STARTED_AT, rideRow, type RideRow } from './rideRows.ts';
import { useRideFeed } from './useRideFeed.ts';

/**
 * RideFeed is what the account has ridden, newest first: the vehicle, the moments the ride ran
 * between, and why it ended. What each ride cost lives in the invoice it links to, because the two
 * collections are ordered by different moments and stitching them together on this screen would make
 * them disagree on the second page.
 */
export function RideFeed({ account, events }: { account: Account; events: CycleFeed }) {
  const feed = useRideFeed(account, events);

  return <FeedList feed={feed} copy={RIDES_ABSENCE} row={(ride) => <RideRowView row={rideRow(ride)} />} />;
}

function RideRowView({ row }: { row: RideRow }) {
  return (
    <>
      <p className="feed-row-title">{row.vehicle}</p>
      <dl className="details">
        <FeedValue term={RIDE_STARTED_AT} value={row.startedAt} />
        <FeedValue term={RIDE_COMPLETED_AT} value={row.completedAt} />
        <FeedValue term={RIDE_REASON} value={row.reason} />
      </dl>
      <Link className="feed-row-link" to={invoiceAddress(row.invoiceId)}>
        {OPEN_INVOICE}
      </Link>
    </>
  );
}
