import { Link } from 'react-router';
import { invoiceAddress } from '../../app/addresses.ts';
import { commandText } from '../../shared/command/commandPhase.ts';
import type { Payment } from '../../shared/command/usePayment.ts';
import { PAID_AT, paidAtText, PAYMENT_STATE, paymentActionText, paymentText } from '../../shared/ride/paymentCopy.ts';
import { amountText } from '../../shared/ride/fares.ts';
import {
  COMPLETION_REASON,
  completionText,
  FINISHED_AT,
  finishedAtText,
  INVOICE_TOTAL,
  RIDE_FINISHED,
} from '../../shared/ride/spell.ts';
import { OPEN_INVOICE } from '../../shared/copy.ts';
import type { CompletedRideCharge, CompletedRideResult } from './completedResult.ts';

/**
 * FinishedRideView is what a person reads about a ride that is over: which vehicle it was, why it
 * ended, when it ended, what the service charged for it and where the payment of that charge stands.
 *
 * Everything it shows comes from the result the service published — the notification about the
 * ending, and the invoice that notification names — so a reload and a new tab show the same summary.
 * The amount and the state of the payment belong to the invoice and are left out while it has not
 * been read, because a total this interface computed itself would not be the total a person is
 * charged.
 *
 * The invoice itself is one link away: what the charge was made of — the minutes of each mode and the
 * rates they were priced at — is the card of that invoice in the cabinet, and this is the only link
 * into the cabinet from outside it.
 */
export function FinishedRideView({ result, paid }: { result: CompletedRideResult; paid: Payment }) {
  const notice = payNotice(paid);
  const charge = result.charge;

  return (
    <div className="reservation-panel-current reservation-panel-finished">
      {result.vehicle !== undefined && <p className="reservation-panel-vehicle">{result.vehicle}</p>}
      <p className="reservation-panel-time" role="status">
        {RIDE_FINISHED}
      </p>

      <dl className="ride-progress">
        <dt>{COMPLETION_REASON}</dt>
        <dd>{completionText(result.completion)}</dd>
        <dt>{FINISHED_AT}</dt>
        <dd>{finishedAtText(result.endedAt)}</dd>
        {charge !== undefined && <ChargeLines charge={charge} />}
      </dl>

      {charge !== undefined && <PayControl charge={charge} paid={paid} />}
      {charge !== undefined && (
        <Link className="feed-row-link" to={invoiceAddress(charge.invoiceId)}>
          {OPEN_INVOICE}
        </Link>
      )}

      {notice !== undefined && <p className="reservation-panel-notice">{notice}</p>}
    </div>
  );
}

/** What the invoice of the ride states: the total, the state of its payment and when it was settled. */
function ChargeLines({ charge }: { charge: CompletedRideCharge }) {
  const paidAt = paidAtText(charge.payment);

  return (
    <>
      <dt>{INVOICE_TOTAL}</dt>
      <dd>{amountText(charge.totalAmountTyiyn)}</dd>
      <dt>{PAYMENT_STATE}</dt>
      <dd>{paymentText(charge.payment)}</dd>
      {paidAt !== undefined && (
        <>
          <dt>{PAID_AT}</dt>
          <dd>{paidAt}</dd>
        </>
      )}
    </>
  );
}

/**
 * The control that pays the invoice. It exists for an invoice the service has not settled, which is
 * what a payment can still change: a settled one would answer the view that already exists.
 */
function PayControl({ charge, paid }: { charge: CompletedRideCharge; paid: Payment }) {
  const action = paymentActionText(charge.payment);
  if (action === undefined) return null;

  const sending = paid.phase.state === 'sending' && paid.phase.action === 'pay';
  return (
    <div className="reservation-panel-actions">
      <button className="action-button" type="button" disabled={sending} onClick={() => paid.pay(charge.invoiceId)}>
        {action}
      </button>
    </div>
  );
}

/** What the panel says about the last payment, if the service refused it. */
function payNotice(paid: Payment): string | undefined {
  if (paid.phase.state !== 'refused') return undefined;
  if (paid.phase.action !== 'pay') return undefined;

  return commandText(paid.phase);
}
