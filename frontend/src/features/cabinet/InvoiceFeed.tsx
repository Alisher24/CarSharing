import { Link } from 'react-router';
import { invoiceAddress } from '../../app/addresses.ts';
import type { Account } from '../../shared/account/session.ts';
import type { CycleFeed } from '../../shared/read/useReadCycle.ts';
import { OPEN_INVOICE } from '../../shared/copy.ts';
import { INVOICES_ABSENCE } from './cabinetCopy.ts';
import { FeedList } from './FeedList.tsx';
import { FeedValue } from './FeedValue.tsx';
import { INVOICE_ISSUED_AT, invoiceRow, type InvoiceRow } from './invoiceRows.ts';
import { PAYMENT_STATE } from '../../shared/ride/paymentCopy.ts';
import { useInvoiceFeed, type InvoiceRecord } from './useInvoiceFeed.ts';

/**
 * InvoiceFeed is what the account has been charged, newest first: when each invoice was issued, what
 * it came to, and where paying it stands. What the charge was made of — the minutes of each mode,
 * the rates they were priced at — is the card of one invoice, which is the one place a person can
 * check the interface against the letter they were sent.
 */
export function InvoiceFeed({ account, events }: { account: Account; events: CycleFeed }) {
  const feed = useInvoiceFeed(account, events);

  return (
    <FeedList
      feed={feed}
      copy={INVOICES_ABSENCE}
      row={(record: InvoiceRecord) => <InvoiceRowView row={invoiceRow(record.view)} />}
    />
  );
}

function InvoiceRowView({ row }: { row: InvoiceRow }) {
  return (
    <>
      <p className="feed-row-title">{row.total}</p>
      <dl className="details">
        <FeedValue term={INVOICE_ISSUED_AT} value={row.issuedAt} />
        <FeedValue term={PAYMENT_STATE} value={row.payment} />
      </dl>
      <Link className="feed-row-link" to={invoiceAddress(row.id)}>
        {OPEN_INVOICE}
      </Link>
    </>
  );
}
