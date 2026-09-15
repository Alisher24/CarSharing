import { useCallback, useSyncExternalStore } from 'react';
import type { ReadCoordinator } from './coordinator.ts';
import type { DocumentKind } from './readCycle.ts';

/** A reader with no coordinator has no requests to watch, so it is never woken by one. */
const noSubscription = () => () => {};

/**
 * useRequestedReads counts the reads the coordinator has asked one document for. Watching the count
 * rather than reading a flag is what makes a request start a read: a request that arrives while one
 * is running stops mattering only once the answer to it is the one on screen.
 */
export function useRequestedReads(
  coordinator: ReadCoordinator | undefined,
  document: DocumentKind | undefined,
): number {
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
