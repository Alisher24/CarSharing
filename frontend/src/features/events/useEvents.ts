import { useCallback, useMemo, useState } from 'react';
import type { Signal } from './changes.ts';
import { useEventStream, type EventsConnection } from './useEventStream.ts';

/** Where the public change stream lives. It is the only stream that carries no session. */
const PUBLIC_EVENTS_URL = '/api/v1/events';

/**
 * What the catalog reads from the stream: how the subscription is doing, which changes have arrived
 * since the last handshake, and how many handshakes there have been.
 */
export type EventsFeed = {
  connection: EventsConnection;
  readyCount: number;
  changes: readonly Signal[];
};

/** One empty list, so a handshake that clears the feed does not look like a changed one. */
const EMPTY: readonly Signal[] = [];

/**
 * useEvents keeps the public change stream open for the whole life of the application. The signals
 * it collects are handed to the catalog, which decides what to read; nothing here reads a snapshot,
 * so a change that arrives while the stream is down costs nothing to miss.
 */
export function useEvents(): EventsFeed {
  const [feed, setFeed] = useState<{ readyCount: number; changes: readonly Signal[] }>({
    readyCount: 0,
    changes: EMPTY,
  });

  // A handshake answers with the resources to read again, so what was collected before it has
  // served its purpose and a reconnect does not leave a stale list behind.
  const onReady = useCallback(() => setFeed((held) => ({ readyCount: held.readyCount + 1, changes: EMPTY })), []);
  const onChanged = useCallback(
    (signal: Signal) => setFeed((held) => ({ readyCount: held.readyCount, changes: [...held.changes, signal] })),
    [],
  );

  const handlers = useMemo(() => ({ onReady, onChanged }), [onReady, onChanged]);
  const connection = useEventStream(PUBLIC_EVENTS_URL, handlers);

  return { connection, readyCount: feed.readyCount, changes: feed.changes };
}
