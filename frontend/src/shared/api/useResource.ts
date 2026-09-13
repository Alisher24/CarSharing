import { useCallback, useEffect, useState, useSyncExternalStore } from 'react';
import { afterFailure, afterSuccess, type Resource } from './Resource.ts';
import type { DocumentKind } from '../../features/events/readCycle.ts';
import type { ReadCoordinator, ReadTicket } from '../../features/events/coordinator.ts';

/** Loads one resource, abandoning the attempt when the signal is aborted. */
export type LoadResource<T> = (signal: AbortSignal) => Promise<T>;

/** One resource together with the way a person asks for it again. */
export type ResourceHandle<T> = { resource: Resource<T>; retry: () => void };

/**
 * How a resource reads itself: the coordinator whose reads it is part of, which document it is,
 * and which session it belongs to. A resource with no coordinator is loaded on its own and only
 * ever replaced by its own later answers.
 */
export type ResourceRead = {
  coordinator: ReadCoordinator;
  document: DocumentKind;

  /** The session this read belongs to. An answer of an ended session is never stored. */
  session: string;
};

/**
 * useResource keeps one resource up to date on its own. Every read it performs is one the
 * coordinator asked for — a change signal, a handshake, reconciliation, or a person retrying —
 * except for the first read of the resource, which is its own. Without a coordinator it is read on
 * the interval it is given.
 *
 * A request is a new read rather than a queued one: the read still in flight is abandoned, so its
 * answer can neither be stored nor compete with the answer to the request that replaced it, and the
 * document is free again the moment it is abandoned, so the read that replaces it does start.
 * Retrying does not clear what is already on screen: the marked-stale snapshot stays until an
 * attempt actually succeeds, and an answer the coordinator no longer accepts leaves the screen as
 * it is rather than replacing it with an older reading.
 */
export function useResource<T>(
  load: LoadResource<T>,
  read?: ResourceRead,
  refreshMilliseconds?: number,
): ResourceHandle<T> {
  const [resource, setResource] = useState<Resource<T>>({ phase: 'loading' });
  const [attempt, setAttempt] = useState(0);

  const session = read?.session;
  const coordinator = read?.coordinator;
  const document = read?.document;
  const requested = useRequestedReads(coordinator, document);

  useEffect(() => {
    const controller = new AbortController();
    let abandoned = false;

    async function reload() {
      const ticket = coordinator === undefined || document === undefined ? undefined : coordinator.begin(document);
      if (coordinator !== undefined && ticket === undefined) return;

      try {
        const value = await load(controller.signal);
        if (abandoned) return;

        const stored = storeAnswer(coordinator, ticket, session, value);
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
    const repeat = refreshMilliseconds ? window.setInterval(() => void reload(), refreshMilliseconds) : undefined;

    return () => {
      abandoned = true;
      controller.abort();
      // The read this effect opened has ended, so the document may be read again: a request that
      // arrived while that answer was on its way is served by the run that follows this one rather
      // than waiting for a read nobody started.
      if (coordinator !== undefined && document !== undefined) coordinator.settle(document);
      if (repeat !== undefined) window.clearInterval(repeat);
    };
  }, [load, coordinator, document, session, requested, refreshMilliseconds, attempt]);

  // A person asking again is a reason to read even when nothing has changed, so the request is
  // registered with the coordinator, which is what the read above watches.
  const retry = useCallback(() => {
    if (coordinator !== undefined && document !== undefined) coordinator.force(document);
    setAttempt((count) => count + 1);
  }, [coordinator, document]);

  return { resource, retry };
}

/** A resource with no coordinator has no requests to watch, so it is never woken by one. */
const noSubscription = () => () => {};

/**
 * useRequestedReads counts the reads the coordinator has asked this document for. Watching the
 * count rather than reading a flag is what makes a request start a read: a request that arrives
 * while one is running stops mattering only once the answer to it is the one on screen.
 */
function useRequestedReads(coordinator: ReadCoordinator | undefined, document: DocumentKind | undefined): number {
  const subscribe = useCallback(
    (listener: () => void) =>
      coordinator === undefined || document === undefined ? noSubscription() : coordinator.watch(document, listener),
    [coordinator, document],
  );
  const asked = useCallback(
    () => (coordinator === undefined || document === undefined ? 0 : coordinator.asked(document)),
    [coordinator, document],
  );
  return useSyncExternalStore(subscribe, asked);
}

/**
 * storeAnswer turns one answer into the resource to hold, or into nothing when the answer must be
 * ignored. An answer that belongs to an ended session, or to a read the coordinator has already
 * answered, is dropped; so is an answer that publishes nothing the screen does not already show.
 */
function storeAnswer<T>(
  coordinator: ReadCoordinator | undefined,
  ticket: ReadTicket | undefined,
  session: string | undefined,
  value: T,
): Resource<T> | undefined {
  if (coordinator === undefined || ticket === undefined || session === undefined) return afterSuccess(value);
  if (!coordinator.accepts(ticket)) return undefined;

  const handlers = coordinator.answer(ticket.document);
  if (!handlers.accepts(session)) return undefined;

  const stored = handlers.observe(value, session);
  if (stored === undefined) return undefined;

  coordinator.cover(ticket.document, stored.covered);
  coordinator.stored(ticket);
  return afterSuccess(stored.value as T);
}
