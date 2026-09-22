import { useEffect } from 'react';
import type { Payment } from './usePayment.ts';

/**
 * useReadAfterPayment reads the invoice again once a payment this tab sent has been answered. The
 * service has stored that answer by the time it arrives, so the invoice is read rather than left
 * showing the state the payment replaced — which is also what keeps the screen right when no change
 * signal reaches this tab.
 */
export function useReadAfterPayment(paid: Payment, read: () => void): void {
  const answered = paid.phase.state === 'done' && paid.phase.action === 'pay';

  useEffect(() => {
    if (answered) read();
  }, [answered, read]);
}
