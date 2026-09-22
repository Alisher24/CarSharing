import { useCallback, useEffect, useState } from 'react';
import { afterFailure, type Resource } from './Resource.ts';
import { storedAnswer } from './answerStore.ts';
import type { ReadCoordinator } from './coordinator.ts';
import type { DocumentKind } from './readCycle.ts';
import { useRequestedReads } from './useRequestedReads.ts';

/** Loads one document, abandoning the attempt when the signal is aborted. */
export type LoadDocument<T> = (signal: AbortSignal) => Promise<T>;

/**
 * How one document reads itself: the coordinator whose reads it is part of, which document it is, and
 * which session it belongs to. A document with no coordinator is loaded on its own and only ever
 * replaced by its own later answers.
 */
export type DocumentRead = {
  coordinator: ReadCoordinator | undefined;
  document: DocumentKind | undefined;

  /** The session this read belongs to. An answer of an ended session is never stored. */
  session: string | undefined;
};

/** What one read of a document asks for, which is what a changed question is. */
export type ReadRequest = {
  /**
   * What one read asks for: the page of a feed a person continued to, or nothing when the document is
   * read whole. A different request is a different read, so the read below is opened again.
   */
  request?: unknown;

  /**
   * How many reads have been asked for, which is a reason to read even with nothing changed: a signal
   * from the cycle, a handshake, reconciliation, or a person retrying. Watching the count rather than
   * a flag is what makes a request start a read — one that arrives while a read is running stops
   * mattering only once the answer to it is the one on screen.
   */
  replay: number;
};

/** One document read through the protocol, together with the way a person asks for it again. */
export type Read<T> = {
  /** The last answer, and how that reading went. */
  resource: Resource<T>;

  /** Reads again on the same terms, which is what a person asks after a failure. */
  repeat: () => void;
};

/**
 * useReadProtocol is the one way a document is read from the service, whether it is a resource read
 * whole or one page of a feed. A read is opened through the coordinator; the answer is stored by
 * `storedAnswer`, so one rule decides what may replace what is on screen; and a read still in flight
 * when the next one starts is abandoned, so its answer can neither be stored nor compete with the
 * answer that replaced it.
 *
 * A read that failed leaves the last successful answer on screen, marked stale: losing what a person
 * is reading is not what one attempt that did not arrive should cost them. A document that has never
 * answered is a failure, because there is nothing to keep.
 *
 * What a read asks for — the loader, and the request it is asked with — is what the read below is
 * opened by, so a resource whose question changed (the invoice a completion names) and a feed whose
 * page changed are read again by the same rule rather than by one each.
 */
export function useReadProtocol<T>(load: LoadDocument<T>, read: DocumentRead, asked: ReadRequest): Read<T> {
  const [resource, setResource] = useState<Resource<T>>({ phase: 'loading' });
  const [attempt, setAttempt] = useState(0);

  const { coordinator, document, session } = read;
  const { request, replay } = asked;
  const requested = useRequestedReads(coordinator, document);

  useEffect(() => {
    const controller = new AbortController();
    let abandoned = false;

    async function reload(): Promise<void> {
      const ticket = coordinator === undefined || document === undefined ? undefined : coordinator.begin(document);
      if (coordinator !== undefined && ticket === undefined) return;

      try {
        const value = await load(controller.signal);
        if (abandoned) return;

        const stored = storedAnswer<T>(coordinator, ticket, session, value);
        if (stored !== undefined) setResource(stored);
      } catch {
        if (!abandoned) setResource(afterFailure);
      } finally {
        // A read this effect abandoned has already been settled by the cleanup that abandoned it, and
        // the read that replaced it owns the document now.
        if (!abandoned && coordinator !== undefined && document !== undefined) coordinator.settle(document);
      }
    }

    void reload();

    return () => {
      abandoned = true;
      controller.abort();
      // The read this effect opened has ended, so the document may be read again: a request that
      // arrived while that answer was on its way is served by the run that follows this one rather
      // than waiting for a read nobody started.
      if (coordinator !== undefined && document !== undefined) coordinator.settle(document);
    };
  }, [load, coordinator, document, session, requested, replay, attempt, request]);

  // A person asking again is a reason to read even when nothing has changed, so the request is
  // registered with the coordinator, which is what the read above watches.
  const repeat = useCallback(() => {
    if (coordinator !== undefined && document !== undefined) coordinator.force(document);
    setAttempt((count) => count + 1);
  }, [coordinator, document]);

  return { resource, repeat };
}
