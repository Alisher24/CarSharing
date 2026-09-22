import { useCallback, useMemo, useState } from 'react';
import type { Signal } from '../../shared/read/changes.ts';
import { useEventStream, type EventsConnection } from '../../shared/read/useEventStream.ts';

/** Where the stream of the signed-in person lives. */
export const PRIVATE_EVENTS_URL = '/api/v1/me/events';

/**
 * What the reader of the private resources takes from the stream: how the subscription is doing,
 * which changes have arrived since the last handshake, and how many handshakes there have been. A
 * change only decides when to read; what is read is the whole resource.
 */
export type PrivateFeed = {
  connection: EventsConnection;
  readyCount: number;
  changes: readonly Signal[];
};

/** One empty list, so a handshake that clears the feed does not look like a changed one. */
const EMPTY: readonly Signal[] = [];

/** What one connection collected, together with the session it was collected for. */
type FeedState = { session: string | undefined; readyCount: number; changes: readonly Signal[] };

/**
 * usePrivateEvents keeps the stream of the signed-in person open while there is one, and closes it
 * the moment there is not. The stream belongs to the session rather than to a screen: opening a
 * panel that shows an account must not open it, and closing one must not stop it.
 *
 * What the connection collected belongs to the session it was collected for. A change is kept only
 * for the session that was open when it arrived, so a signal of a previous account is never handed
 * to the reader of the next one.
 *
 * A subscription that has ended asks the session to be checked again, because a private stream ends
 * for one of two reasons and only the server can say which: the session is gone, or the connection
 * was. An answer that nobody is signed in clears the session, and with it the token that opened
 * this stream, so no reconnection is attempted until a session exists again.
 */
export function usePrivateEvents(session: string | undefined, onEnded: () => void): PrivateFeed {
  const [feed, setFeed] = useState<FeedState>({ session, readyCount: 0, changes: EMPTY });

  // A handshake answers with the resources to read again, which is also what a reconnect needs: the
  // changes collected before it describe a connection that no longer exists.
  const onReady = useCallback(() => {
    setFeed((held) =>
      held.session === session
        ? { session, readyCount: held.readyCount + 1, changes: EMPTY }
        : { session, readyCount: 1, changes: EMPTY },
    );
  }, [session]);

  const onChanged = useCallback(
    (signal: Signal) =>
      setFeed((held) =>
        held.session === session ? { session, readyCount: held.readyCount, changes: [...held.changes, signal] } : held,
      ),
    [session],
  );

  const handlers = useMemo(() => ({ onReady, onChanged, onEnded }), [onReady, onChanged, onEnded]);
  const connection = useEventStream(session === undefined ? undefined : PRIVATE_EVENTS_URL, handlers);

  // A session that is not the one that collected this feed has collected nothing: the reader of the
  // new session starts with an empty list rather than with another account's history.
  const current = feed.session === session ? feed : { session, readyCount: 0, changes: EMPTY };
  return { connection, readyCount: current.readyCount, changes: current.changes };
}
