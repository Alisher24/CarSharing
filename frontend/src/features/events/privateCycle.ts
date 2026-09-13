import { useEffect } from 'react';
import { changedResources, type Signal } from './changes.ts';
import type { ReadCoordinator } from './coordinator.ts';
import { RECONCILE_MILLISECONDS, reconciliationEnabled } from './reconciliation.ts';
import type { DocumentKind } from './readCycle.ts';

/**
 * How long a batch of private changes is collected before it is read, which is the window the
 * catalog uses for its own: a burst of changes is one read rather than one per change.
 */
const COALESCE_MILLISECONDS = 100;

/** What a reader takes from the private stream: the changes since the last handshake, and how many there were. */
export type PrivateFeed = { changes: readonly Signal[]; readyCount: number };

/**
 * usePrivateCycle is the place of one private reader in the read cycle: a change asks for the
 * documents it names, a handshake asks for every document the reader holds, and the interval asks
 * for them again so that a signal nobody received is repaired by reading. Nothing here reads
 * anything itself — a reader is woken by a request and decides what to read — so one cycle serves
 * the reservation and the notifications without either deciding anything for the other.
 *
 * A reader with no session has no coordinator and takes no part in any of it.
 */
export function usePrivateCycle(
  coordinator: ReadCoordinator | undefined,
  feed: PrivateFeed,
  documents: readonly DocumentKind[],
): void {
  useChanges(coordinator, feed.changes);
  useHandshake(coordinator, feed.readyCount, documents);
  useReconciliation(coordinator, documents);
}

/**
 * Feeds the changes one private connection delivered into the cycle. Each change is remembered at
 * once, so one that arrives during a request is not lost, and the reads they ask for are opened
 * together once the window has passed.
 */
function useChanges(coordinator: ReadCoordinator | undefined, changes: readonly Signal[]): void {
  useEffect(() => {
    if (coordinator === undefined) return;

    for (const change of changes) coordinator.observe(change.resource, change.id, change.version);

    const batch = window.setTimeout(() => askFor(coordinator, changedResources(changes)), COALESCE_MILLISECONDS);
    return () => window.clearTimeout(batch);
  }, [coordinator, changes]);
}

/** Asks for a read of each document a window named. */
function askFor(coordinator: ReadCoordinator, documents: readonly DocumentKind[]): void {
  for (const document of documents) {
    if (coordinator.due(document)) coordinator.force(document);
  }
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
 * Asks for every document the reader holds on the same interval the catalog uses. This is what
 * repairs a signal that never arrived, a stream that was down while the change happened, and the
 * day's allowance being restored at local midnight, which no signal announces.
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
