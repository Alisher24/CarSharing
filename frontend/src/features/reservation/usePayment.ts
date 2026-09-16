import { useEffect } from 'react';
import { csrfTokenOf, sessionOf, type Account } from '../account/useAccount.ts';
import { payInvoice } from '../../shared/api/invoices.ts';
import { useCommandSender } from './commandSender.ts';
import type { CommandPhase } from './commandPhase.ts';

/** What the interface can do about one invoice of the account. */
export type Payment = {
  /** Where the payment the interface last sent stands. */
  phase: CommandPhase;

  /** Sends one payment for one invoice, with a key drawn now. */
  pay: (invoiceId: string) => void;

  /** Forgets what the last payment answered, so a message is not shown twice. */
  settle: () => void;
};

/**
 * usePayment sends the one payment a person makes themselves. It is the same command wherever it is
 * asked for — below the ride that has just ended, and on the card of an invoice from the history —
 * so it knows only the invoice it settles: an invoice read from the history has no ride on screen to
 * be part of, and the command must not depend on one.
 *
 * What the screen shows is the state the read of the invoice publishes, and that read is asked for
 * again by useReadAfterPayment once the service has answered: what is settled here is the command,
 * not the state of the invoice.
 *
 * Every ask carries a new key. A repeat of the key of a refused attempt would answer that stored
 * refusal, which is a repeat of the answer rather than another payment.
 */
export function usePayment(account: Account): Payment {
  const { phase, start, settle } = useCommandSender({
    owner: sessionOf(account),
    csrfToken: csrfTokenOf(account),
    // The payment moves nothing about the rental: what is current is not read again for it.
    refresh: () => undefined,
    send: (command, credentials) => payInvoice(command.parameters.invoiceId ?? '', credentials),
  });

  return {
    phase,
    pay: (invoiceId) => void start('pay', { invoiceId }),
    settle,
  };
}

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
