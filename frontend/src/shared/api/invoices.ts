import { getInvoice, payInvoice as payInvoiceRequest } from './generated/sdk.gen';
import {
  answerOf,
  commandHeaders,
  sameOriginRequest,
  type CommandCredentials,
  type CommandResult,
} from './commands.ts';
import type { InvoiceView, PayResult } from './generated/types.gen';

export type { Invoice, InvoiceView, PayResult, Payment } from './generated/types.gen';

/**
 * fetchInvoice reads one invoice of the caller's own account together with the state of its payment.
 * It is a private read: the browser sends the session cookie, and an invoice of another account is
 * answered as absent rather than as forbidden.
 *
 * What it publishes is what a completed ride is shown from — the completion the invoice was issued
 * for, the exact total, and where paying it stands — and no other answer the interface reads carries
 * the state of a payment, which the service moves on its own.
 */
export async function fetchInvoice(invoiceId: string, signal: AbortSignal): Promise<InvoiceView> {
  const { data } = await getInvoice({ path: { id: invoiceId }, ...sameOriginRequest, throwOnError: true, signal });
  return data;
}

/**
 * The command that settles an invoice: the one transition of a payment a person makes themselves,
 * which the service offers after its own first attempt was refused.
 *
 * The command carries a key of its own at every ask. A repeat of the key of a refused attempt would
 * answer that stored refusal rather than paying, so asking again is a new command rather than a
 * repeat of the previous one — the opposite of what a repeat of a command whose answer never arrived
 * is for.
 */
export async function payInvoice(
  invoiceId: string,
  credentials: CommandCredentials,
): Promise<CommandResult<PayResult>> {
  return answerOf(() =>
    payInvoiceRequest({
      path: { id: invoiceId },
      headers: commandHeaders(credentials),
      ...sameOriginRequest,
    }),
  );
}
