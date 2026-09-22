import { useCallback } from 'react';
import { payInvoice } from '../api/invoices.ts';
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
export function usePayment(owner: string | undefined, csrfToken: string | undefined): Payment {
  const { phase, start, settle } = useCommandSender({
    owner,
    csrfToken,
    // The payment moves nothing about the rental: what is current is not read again for it.
    refresh: () => undefined,
    send: (command, credentials) => payInvoice(command.parameters.invoiceId ?? '', credentials),
  });

  const pay = useCallback((invoiceId: string) => void start('pay', { invoiceId }), [start]);

  return { phase, pay, settle };
}
