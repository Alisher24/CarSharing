import type { FinishResult } from '../../shared/api/current.ts';
import {
  COMPLETION_REASON,
  completionText,
  FINISHED_AT,
  invoiceTotalText,
  INVOICE_TOTAL,
  RIDE_FINISHED,
} from './rideCopy.ts';

/**
 * FinishedRideView is what a person reads about a ride that is over: which vehicle it was, why it
 * ended, when it ended and what the service charged for it. Everything it shows comes from the answer
 * the finish command produced, so the amount on screen is the amount of the invoice rather than an
 * estimate the interface computed again.
 *
 * Nothing here records anything. The answer was written down by the handler that received it, which is
 * why a reload shows the same summary rather than nothing.
 */
export function FinishedRideView({ finished }: { finished: FinishResult }) {
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
      </dl>
    </div>
  );
}
