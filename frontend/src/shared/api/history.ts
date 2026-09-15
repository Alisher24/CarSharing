import { getInvoices, getRides } from './generated/sdk.gen';
import type { InvoiceCollection, RideCollection } from './generated/types.gen';

export type { InvoiceCollection, RideCollection, RideSummary, VehicleReference } from './generated/types.gen';

// Both collections belong to one account: the browser sends the session cookie, and the service
// answers the rides and the invoices of the caller rather than of anybody the request could name.
const sameOriginRequest = { credentials: 'same-origin', cache: 'no-store' } as const;

/**
 * fetchRides reads one page of the rides the caller has finished, newest first. A cursor continues
 * the page it was handed back with; without one the newest page is read.
 *
 * No page size is sent. A cursor is bound to the parameters it was issued under, so a continuation
 * that states no number cannot disagree with the size the first page was read at, and the service's
 * own default is the one size both pages use.
 */
export async function fetchRides(cursor: string | undefined, signal: AbortSignal): Promise<RideCollection> {
  const { data } = await getRides({ query: cursorQuery(cursor), ...sameOriginRequest, throwOnError: true, signal });
  return data;
}

/**
 * fetchInvoices reads one page of the caller's own invoices, newest first, each with the state of
 * its payment. It is ordered by the moment each invoice was issued, which is not the order the rides
 * are in: the two collections are read side by side rather than stitched together.
 */
export async function fetchInvoices(cursor: string | undefined, signal: AbortSignal): Promise<InvoiceCollection> {
  const { data } = await getInvoices({ query: cursorQuery(cursor), ...sameOriginRequest, throwOnError: true, signal });
  return data;
}

/** The query one page is asked for with: the cursor of a continuation, or nothing for the newest. */
function cursorQuery(cursor: string | undefined): { cursor?: string } {
  return cursor === undefined ? {} : { cursor };
}
