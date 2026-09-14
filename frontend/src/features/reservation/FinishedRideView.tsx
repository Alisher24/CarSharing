import type { FinishResult, InvoiceView } from '../../shared/api/current.ts';
import { commandText } from './commandPhase.ts';
import { PAID_AT, PAYMENT_STATE, paidAtText, paymentActionText, paymentText } from './paymentCopy.ts';
import {
  COMPLETION_REASON,
  completionText,
  FINISHED_AT,
  invoiceTotalText,
  INVOICE_TOTAL,
  RIDE_FINISHED,
} from './rideCopy.ts';
import type { Payment } from './usePayment.ts';

/**
 * FinishedRideView is what a person reads about a ride that is over: which vehicle it was, why it
 * ended, when it ended, what the service charged for it and where the payment of that charge stands.
 * Everything it shows comes from the answer the finish command produced and from the answers to the
 * payments since, so the amount on screen is the amount of the invoice rather than an estimate the
 * interface computed again.
 *
 * Nothing here records anything. Both answers were written down by the handler that received them,
 * which is why a reload shows the same summary rather than nothing.
 */
export function FinishedRideView({
  finished,
  payment,
  paid,
}: {
  finished: FinishResult;
  payment: InvoiceView;
  paid: Payment;
}) {
  const sending = paid.phase.state === 'sending' && paid.phase.action === 'pay';
  const notice = paid.phase.state === 'refused' && paid.phase.action === 'pay' ? commandText(paid.phase) : undefined;
  const action = paymentActionText(payment.payment);
  const paidAt = payment.payment.status === 'paid' ? paidAtText(payment) : undefined;

  return (
    <div className="reservation-panel-current reservation-panel-finished">
      <p className="reservation-panel-vehicle">{finished.rental.vehicle.model}</p>
      <p className="reservation-panel-time" role="status">
        {RIDE_FINISHED}
      </p>

      <dl className="ride-progress">
        <dt>{COMPLETION_REASON}</dt>
        <dd>{completionText(finished)}</dd>
        <dt>{FINISHED_AT}</dt>
        <dd>{finished.rental.completed_at}</dd>
        <dt>{INVOICE_TOTAL}</dt>
        <dd>{invoiceTotalText(finished)}</dd>
        <dt>{PAYMENT_STATE}</dt>
        <dd>{paymentText(payment.payment)}</dd>
        {paidAt !== undefined && (
          <>
            <dt>{PAID_AT}</dt>
            <dd>{paidAt}</dd>
          </>
        )}
      </dl>

      {/* The control exists for an invoice the service has not settled, which is what a payment can
          still change: a settled one would answer the view that already exists. */}
      {action !== undefined && (
        <div className="reservation-panel-actions">
          <button
            className="action-button"
            type="button"
            disabled={sending}
            onClick={() => paid.pay(finished.invoice.invoice.id)}
          >
            {action}
          </button>
        </div>
      )}

      {notice !== undefined && <p className="reservation-panel-notice">{notice}</p>}
    </div>
  );
}
