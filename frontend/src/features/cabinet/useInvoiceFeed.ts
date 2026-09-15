import type { InvoiceView } from '../../shared/api/current.ts';
import { fetchInvoices } from '../../shared/api/history.ts';
import { sessionOf, type Account } from '../account/useAccount.ts';
import type { DocumentKind } from '../events/readCycle.ts';
import type { PrivateFeed } from '../events/usePrivateEvents.ts';
import { isNewerVersion } from '../events/version.ts';
import type { Replaces } from './feedPages.ts';
import { useFeed, type Feed, type ReadFeedPage } from './useFeed.ts';

/** The document a change to one of the account's invoices is signalled under. */
const INVOICES_DOCUMENT: DocumentKind = 'invoices';

/**
 * One invoice of the feed. A view of an invoice is addressed by the invoice it is a view of, which
 * is what a merge matches two readings of the same invoice by; the view itself is what a row and a
 * card are built from.
 */
export type InvoiceRecord = { id: string; view: InvoiceView };

/**
 * An invoice never changes, but the payment published beside it does, and the version of the view is
 * what says which of two readings is the later one. A reading that is not newer than the one on
 * screen is refused, so an answer that overtook another cannot put a settled payment back to
 * pending.
 */
const NEWER_VIEW: Replaces<InvoiceRecord> = (held, answered) =>
  isNewerVersion(answered.view.version, held.view.version);

const readInvoicePage: ReadFeedPage<InvoiceRecord> = (cursor, signal) =>
  fetchInvoices(cursor, signal).then((page) => ({
    records: page.items.map(invoiceRecord),
    nextCursor: page.next_cursor,
  }));

/** useInvoiceFeed reads the signed-in person's own invoices, newest first. */
export function useInvoiceFeed(account: Account, events: PrivateFeed): Feed<InvoiceRecord> {
  return useFeed<InvoiceRecord>({
    session: sessionOf(account),
    document: INVOICES_DOCUMENT,
    events,
    read: readInvoicePage,
    replaces: NEWER_VIEW,
  });
}

function invoiceRecord(view: InvoiceView): InvoiceRecord {
  return { id: view.invoice.id, view };
}
