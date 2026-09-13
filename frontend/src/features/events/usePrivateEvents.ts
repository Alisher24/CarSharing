import { useCallback, useMemo } from 'react';
import type { Signal } from './changes.ts';
import { useEventStream, type EventsConnection } from './useEventStream.ts';

/** Where the stream of the signed-in person lives. */
export const PRIVATE_EVENTS_URL = '/api/v1/me/events';

/**
 * usePrivateEvents keeps the stream of the signed-in person open while there is one, and closes it
 * the moment there is not. The stream belongs to the session rather than to a screen: opening a
 * panel that shows an account must not open it, and closing one must not stop it.
 *
 * A subscription that has ended asks the session to be checked again, because a private stream ends
 * for one of two reasons and only the server can say which: the session is gone, or the connection
 * was. An answer that nobody is signed in clears the session, and with it the token that opened
 * this stream, so no reconnection is attempted until a session exists again.
 */
export function usePrivateEvents(
  session: boolean,
  onChanged: (signal: Signal) => void,
  onEnded: () => void,
): EventsConnection {
  const onReady = useCallback(() => undefined, []);
  const handlers = useMemo(() => ({ onReady, onChanged, onEnded }), [onReady, onChanged, onEnded]);

  return useEventStream(session ? PRIVATE_EVENTS_URL : undefined, handlers);
}
