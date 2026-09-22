import { useCallback, useEffect } from 'react';
import type { Resource } from './Resource.ts';
import { useReadProtocol, type DocumentRead, type LoadDocument } from './useReadProtocol.ts';
import { useRequestedReads } from './useRequestedReads.ts';

/** One resource together with the way a person asks for it again. */
export type ResourceHandle<T> = { resource: Resource<T>; retry: () => void };

/**
 * How a resource reads itself, for a view that holds one value whole: the coordinator whose reads it
 * is part of, which document it is, and which session it belongs to.
 */
export type ResourceRead = {
  coordinator: DocumentRead['coordinator'];
  document: DocumentRead['document'];
  session: DocumentRead['session'];
};

/**
 * useResource keeps one resource up to date on its own. Every read it performs is one the coordinator
 * asked for — a change signal, a handshake, reconciliation, or a person retrying — except for the
 * first read of the resource, which is its own. Without a coordinator it is read on the interval it is
 * given, and by a person.
 *
 * Retrying does not clear what is already on screen: the marked-stale answer stays until an attempt
 * actually succeeds, and an answer the coordinator no longer accepts leaves the screen as it is
 * rather than replacing it with an older reading. How a read is opened, abandoned and stored is
 * `useReadProtocol`, which a feed reads its pages through as well.
 */
export function useResource<T>(
  load: LoadDocument<T>,
  read?: ResourceRead,
  refreshMilliseconds?: number,
): ResourceHandle<T> {
  const asked = read ?? { coordinator: undefined, document: undefined, session: undefined };
  const requested = useRequestedReads(asked.coordinator, asked.document);
  const { resource, repeat } = useReadProtocol<T>(load, asked, { replay: requested });

  // A resource that is read on its own clock rather than by the cycle around it.
  useEffect(() => {
    if (refreshMilliseconds === undefined) return undefined;

    const interval = window.setInterval(repeat, refreshMilliseconds);
    return () => window.clearInterval(interval);
  }, [refreshMilliseconds, repeat]);

  const retry = useCallback(() => repeat(), [repeat]);

  return { resource, retry };
}

export type { DocumentRead, LoadDocument } from './useReadProtocol.ts';
