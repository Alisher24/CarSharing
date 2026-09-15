import { useCallback, useMemo } from 'react';
import type { InvoiceView } from '../../shared/api/current.ts';
import { fetchInvoice } from '../../shared/api/invoices.ts';
import type { Resource } from '../../shared/api/Resource.ts';
import { useResource, type ResourceRead } from '../../shared/api/useResource.ts';
import { sessionOf, type Account } from '../account/useAccount.ts';
import { answersOf } from '../events/documentAnswers.ts';
import { createReadCoordinator, type ReadCoordinator } from '../events/coordinator.ts';
import { usePrivateCycle } from '../events/privateCycle.ts';
import type { AnswerHandlers, DocumentKind } from '../events/readCycle.ts';
import type { PrivateFeed } from '../events/usePrivateEvents.ts';
import { isNewerVersion } from '../events/version.ts';

/** The one document the reader of a single invoice keeps up to date. */
const INVOICE_DOCUMENTS: readonly DocumentKind[] = ['invoices'];

/** What a person reads about one invoice of their own, and the way to ask for it again. */
export type InvoiceCard = {
  /** The last reading of the invoice, and how that reading went. */
  resource: Resource<InvoiceView | undefined>;

  /** Reads the invoice again, which is what a person asks for after a failed read. */
  retry: () => void;
};

/**
 * useInvoiceCard reads one invoice of the signed-in person and keeps it up to date from the private
 * stream and from its own reconciliation: the payment beside an invoice moves without this tab doing
 * anything, and the change that moves it is announced under the account's invoices.
 *
 * An invoice of another account and one that never existed are both answered as absent by the
 * service, so the failure to read them is the same failure here: nothing tells a person that
 * somebody else's invoice exists.
 */
export function useInvoiceCard(account: Account, invoiceId: string, events: PrivateFeed): InvoiceCard {
  const session = sessionOf(account);

  const state = useMemo(() => ({ version: undefined as string | undefined }), [session, invoiceId]);
  const coordinator = useMemo(() => coordinatorFor(session, state), [session, state]);
  const read = useMemo(
    () => (coordinator === undefined || session === undefined ? undefined : readOf(coordinator, session)),
    [coordinator, session],
  );

  const load = useCallback(
    (signal: AbortSignal) => (session === undefined ? Promise.resolve(undefined) : fetchInvoice(invoiceId, signal)),
    [session, invoiceId],
  );
  const handle = useResource<InvoiceView | undefined>(load, read);

  usePrivateCycle(coordinator, events, INVOICE_DOCUMENTS);

  return { resource: handle.resource, retry: handle.retry };
}

/** What one session of reading one invoice knows: the version of the view already on screen. */
type CardSession = { version: string | undefined };

/** The reader of one session, which carries the version only that session may compare against. */
function coordinatorFor(session: string | undefined, state: CardSession): ReadCoordinator | undefined {
  if (session === undefined) return undefined;

  return createReadCoordinator(session, answersOf({ invoices: invoiceAnswer(session, state) }));
}

/**
 * The answer of one invoice. A reading that is not newer than the one on screen is refused, so an
 * answer that overtook another cannot put a settled payment back to the state it replaced.
 */
function invoiceAnswer(session: string, state: CardSession): AnswerHandlers {
  return {
    accepts: (answering) => answering === session,
    observe: (value, answering) => {
      if (answering !== session) return undefined;

      const view = value as InvoiceView | undefined;
      if (view === undefined) return undefined;
      if (state.version !== undefined && !isNewerVersion(view.version, state.version)) return undefined;

      state.version = view.version;
      // The card reads one invoice, and the signal it is read on names that invoice: covering the
      // version it publishes is what stops the cycle asking about a change this reading answered.
      return { value: view, covered: new Map([[view.invoice.id, view.version]]) };
    },
  };
}

function readOf(coordinator: ReadCoordinator, session: string): ResourceRead {
  return { coordinator, document: 'invoices', session };
}
