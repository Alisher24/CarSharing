import type { Account } from '../account/useAccount.ts';
import { storePayment } from './completedRide.ts';
import { payInvoice } from '../../shared/api/invoices.ts';
import { useCommandSender, type CommandAnswer } from './commandSender.ts';
import type { UnfinishedCommand } from './unfinishedCommand.ts';
import type { CommandPhase } from './commandPhase.ts';

/** What the interface can do about the invoice of a ride that has ended. */
export type Payment = {
  /** Where the payment the interface last sent stands. */
  phase: CommandPhase;

  /** Sends one payment for one invoice, with a key drawn now. */
  pay: (invoiceId: string) => void;

  /** Forgets what the last payment answered, so a message is not shown twice. */
  settle: () => void;
};

/**
 * usePayment sends the one payment a person makes themselves and keeps the state of the invoice it
 * settled. It is the sender the finished ride uses, told a different command: the same key is kept
 * before the request and the same answer is applied where it arrives.
 *
 * The answer is written into the record of the ending rather than into a state of its own, so a reload
 * of the tab shows the payment the service confirmed instead of the one it replaced. Nothing is read
 * again after the answer: the contract publishes no read of an invoice, and the answer to the payment
 * is the state of it.
 *
 * Every ask carries a new key. A repeat of the key of a refused attempt would answer that stored
 * refusal, which is a repeat of the answer rather than another payment.
 */
export function usePayment(account: Account): Payment {
  const owner = account.state === 'signed-in' ? account.snapshot.user.id : undefined;

  const { phase, start, settle } = useCommandSender({
    owner,
    csrfToken: account.state === 'signed-in' ? account.snapshot.csrf_token : undefined,
    // The payment moves nothing about the rental: what is current is not read again for it.
    refresh: () => undefined,
    send: (command, credentials) => settling(command, credentials, owner),
  });

  return {
    phase,
    pay: (invoiceId) => void start('pay', { invoiceId }),
    settle,
  };
}

/**
 * One payment, which keeps the state the service answered with in the record of the ending it belongs
 * to. The record is written here, where the answer arrived, rather than by the render that shows it.
 */
async function settling(
  command: UnfinishedCommand,
  credentials: { csrfToken: string; key: string },
  owner: string | undefined,
): Promise<CommandAnswer> {
  const invoiceId = command.parameters.invoiceId ?? '';
  const answer = await payInvoice(invoiceId, credentials);
  if (answer.outcome === 'done' && owner !== undefined) {
    storePayment(owner, invoiceId, answer.answer.invoice, Date.now());
  }

  return answer;
}
