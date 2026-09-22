import { useEffect } from 'react';
import { changedResources, type Signal } from './changes.ts';
import type { ReadCoordinator } from './coordinator.ts';
import { RECONCILE_MILLISECONDS, reconciliationEnabled } from './reconciliation.ts';
import type { DocumentKind } from './readCycle.ts';
import { requestDocuments } from './request.ts';

/**
 * How long a batch of changes is collected before it is read. One window costs one read of each
 * document it named, so a burst of changes to one resource is one read rather than one per change.
 */
export const COALESCE_MILLISECONDS = 100;

/** What a reader takes from a change stream: the changes since the last handshake, and how many there were. */
export type CycleFeed = { changes: readonly Signal[]; readyCount: number };

/**
 * useReadCycle is the place of one reader in the read cycle: a change asks for the documents it
 * names, a handshake asks for every document the reader holds, and the interval asks for them again
 * so that a signal nobody received is repaired by reading. Nothing here reads anything itself — a
 * reader is woken by a request and decides what to read — so one cycle serves the catalog, the
 * reservation, the notifications and the feeds of the cabinet without any of them deciding anything
 * for another.
 *
 * A reader with no coordinator has no documents to keep up to date and takes no part in any of it.
 */
export function useReadCycle(
  coordinator: ReadCoordinator | undefined,
  feed: CycleFeed,
  documents: readonly DocumentKind[],
): void {
  useChanges(coordinator, feed.changes);
  useHandshake(coordinator, feed.readyCount, documents);
  useReconciliation(coordinator, documents);
}

/**
 * Feeds the changes one connection delivered into the cycle. Each change is remembered at once, so
 * one that arrives during a request is not lost, and the reads they ask for are opened together
 * once the window has passed.
 */
function useChanges(coordinator: ReadCoordinator | undefined, changes: readonly Signal[]): void {
  useEffect(() => {
    if (coordinator === undefined) return;

    for (const change of changes) coordinator.observe(change.resource, change.id, change.version);

    const batch = window.setTimeout(
      () => requestDocuments(coordinator, changedResources(changes)),
      COALESCE_MILLISECONDS,
    );
    return () => window.clearTimeout(batch);
  }, [coordinator, changes]);
}

/** Answers a handshake by asking for every document the reader holds, reconnects included. */
function useHandshake(
  coordinator: ReadCoordinator | undefined,
  readyCount: number,
  documents: readonly DocumentKind[],
): void {
  useEffect(() => {
    if (coordinator !== undefined && readyCount > 0) coordinator.request(documents);
  }, [coordinator, readyCount, documents]);
}

/**
 * Asks for every document the reader holds on a fixed interval. This is the one request that does
 * not depend on anything having changed, because its whole purpose is to find the changes nothing
 * told this client about — a signal the stream lost, a stream that was down while it happened, or
 * the day's allowance being restored at local midnight, which no signal announces.
 */
function useReconciliation(coordinator: ReadCoordinator | undefined, documents: readonly DocumentKind[]): void {
  useEffect(() => {
    if (coordinator === undefined) return;

    const repeat = window.setInterval(() => {
      if (reconciliationEnabled()) coordinator.request(documents);
    }, RECONCILE_MILLISECONDS);
    return () => window.clearInterval(repeat);
  }, [coordinator, documents]);
}
