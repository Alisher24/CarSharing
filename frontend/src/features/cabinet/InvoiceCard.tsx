import { Link, useParams } from 'react-router';
import { INVOICE_PARAMETER, INVOICES_ADDRESS } from '../../app/addresses.ts';
import type { InvoiceView } from '../../shared/api/current.ts';
import { identityOf } from '../../shared/account/identity.ts';
import type { Account } from '../../shared/account/session.ts';
import { commandText } from '../../shared/command/commandPhase.ts';
import { usePayment, type Payment } from '../../shared/command/usePayment.ts';
import { useReadAfterPayment } from '../../shared/command/useReadAfterPayment.ts';
import { ResourceNotice } from '../../shared/components/ResourceNotice.tsx';
import { loadedValue } from '../../shared/read/Resource.ts';
import { found } from '../../shared/read/presence.ts';
import type { CycleFeed } from '../../shared/read/useReadCycle.ts';
import { PAID_AT, paidAtText, PAYMENT_STATE, paymentActionText, paymentText } from '../../shared/ride/paymentCopy.ts';
import { COMPLETION_REASON, completionText } from '../../shared/ride/spell.ts';
import { BACK_TO_INVOICES } from '../../shared/copy.ts';
import { INVOICE_ABSENCE } from './cabinetCopy.ts';
import { FeedValue } from './FeedValue.tsx';
import { INVOICE_ISSUED_AT, LINE_AMOUNT, LINE_MINUTES, LINE_RATE, invoiceLineRows, invoiceRow } from './invoiceRows.ts';
import type { InvoiceLineRow } from './invoiceRows.ts';
import { useInvoiceCard } from './useInvoiceCard.ts';

/**
 * InvoiceCard is one invoice of the account in full: the two lines the charge was made of, with the
 * minutes begun in each mode, the rate they were priced at and what each came to, the total, and
 * where paying it stands.
 *
 * This is the one screen a person can check the interface against the letter the service sent them,
 * which is why it states every number the letter states rather than a summary of them. An invoice
 * nothing has settled offers the payment, because a debt refuses the next reservation and closing it
 * must not depend on the panel of the last ride still being on screen.
 */
export function InvoiceCard({ account, events }: { account: Account; events: CycleFeed }) {
  const invoiceId = useParams()[INVOICE_PARAMETER] ?? '';
  const card = useInvoiceCard(account, invoiceId, events);
  const { session, csrfToken } = identityOf(account);
  const paid = usePayment(session, csrfToken);
  useReadAfterPayment(paid, card.retry);

  const view = loadedValue(card.resource);

  return (
    <article className="invoice-card">
      {view !== undefined && <InvoiceDetails view={view} paid={paid} />}
      <ResourceNotice found={found(card.resource, view)} copy={INVOICE_ABSENCE} onRetry={card.retry} />
      <Link className="feed-row-link" to={INVOICES_ADDRESS}>
        {BACK_TO_INVOICES}
      </Link>
    </article>
  );
}

/**
 * What the invoice states, which is every number of the letter it came from: the total, which heads
 * the card and is stated nowhere else, the moment it was issued, why the ride ended, where paying it
 * stands, and the lines it was charged of. One amount of one invoice is written once on the screen a
 * person checks it against the letter.
 */
function InvoiceDetails({ view, paid }: { view: InvoiceView; paid: Payment }) {
  const row = invoiceRow(view);
  const paidAt = paidAtText(view.payment);

  return (
    <>
      <p className="invoice-card-total">{row.total}</p>
      <dl className="details">
        <FeedValue term={INVOICE_ISSUED_AT} value={row.issuedAt} />
        <FeedValue term={COMPLETION_REASON} value={completionText(view.invoice.completion)} />
        <FeedValue term={PAYMENT_STATE} value={paymentText(view.payment)} />
        {paidAt !== undefined && <FeedValue term={PAID_AT} value={paidAt} />}
      </dl>

      {invoiceLineRows(view).map((line) => (
        <ChargeLine key={line.mode} line={line} />
      ))}

      <PayControl view={view} paid={paid} />
    </>
  );
}

/** One line of the charge: the mode, the minutes begun in it, the rate they cost and their product. */
function ChargeLine({ line }: { line: InvoiceLineRow }) {
  return (
    <section className="invoice-line" aria-label={line.mode}>
      <h3 className="invoice-line-mode">{line.mode}</h3>
      <dl className="details">
        <FeedValue term={LINE_MINUTES} value={line.minutes} />
        <FeedValue term={LINE_RATE} value={line.rate} />
        <FeedValue term={LINE_AMOUNT} value={line.amount} />
      </dl>
    </section>
  );
}

/**
 * The control that pays the invoice, and what the service last answered about a payment sent from
 * here. It exists for an invoice the service has not settled, which is what a payment can still
 * change: a settled one would answer the view that already exists.
 */
function PayControl({ view, paid }: { view: InvoiceView; paid: Payment }) {
  const action = paymentActionText(view.payment);
  if (action === undefined) return null;

  const sending = paid.phase.state === 'sending' && paid.phase.action === 'pay';
  const refused = paid.phase.state === 'refused' && paid.phase.action === 'pay';
  return (
    <>
      <button className="action-button" type="button" disabled={sending} onClick={() => paid.pay(view.invoice.id)}>
        {action}
      </button>
      {refused && <p className="reservation-panel-notice">{commandText(paid.phase)}</p>}
    </>
  );
}
