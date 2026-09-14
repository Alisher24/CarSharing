import { payInvoice as payInvoiceRequest } from './generated/sdk.gen';
import {
  answerOf,
  commandHeaders,
  sameOriginRequest,
  type CommandCredentials,
  type CommandResult,
} from './commands.ts';
import type { PayResult } from './generated/types.gen';

export type { Invoice, InvoiceView, PayResult, Payment } from './generated/types.gen';

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
