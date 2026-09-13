import { useEffect, useRef, useState } from 'react';
import type { Signal } from './changes.ts';
import { streamFrames } from './stream.ts';
import { streamEvent, type StreamEvent } from './streamEvents.ts';

/**
 * What the interface knows about the change stream. Connecting covers the first subscription and
 * every reconnect: a subscription is not established until the server has said ready, so an open
 * response is not yet a working stream.
 */
export type EventsConnection = 'connecting' | 'connected' | 'delayed';

/** What one frame of the stream is turned into while the stream stays open. */
export type StreamHandlers = {
  onReady: () => void;
  onChanged: (signal: Signal) => void;

  /** That a subscription has ended, which is when the session behind it has to be checked again. */
  onEnded?: () => void;
};

/**
 * How long a client waits before subscribing again. The server states this number in its handshake,
 * and the client uses it as its own constant: the reconnect delay is part of the contract both sides
 * are built from rather than something one of them may vary at runtime.
 */
const DECLARED_RETRY_MILLISECONDS = 3000;

/**
 * useEventStream keeps one subscription to the change stream alive for as long as it is mounted.
 * A connection the server or the network ended is retried after the declared delay; recovering the
 * state a missed signal described is the caller's business, because it is done by reading REST.
 */
export function useEventStream(url: string | undefined, handlers: StreamHandlers): EventsConnection {
  const latest = useRef(handlers);
  const [connection, setConnection] = useState<EventsConnection>('connecting');

  // The handlers are read when a frame arrives rather than captured with the connection, so a
  // caller re-rendering does not tear down and rebuild a subscription that is working.
  useEffect(() => {
    latest.current = handlers;
  }, [handlers]);

  useEffect(() => {
    if (url === undefined) return undefined;

    const controller = new AbortController();
    setConnection('connecting');
    void follow(url, controller.signal, latest, setConnection).catch(() => undefined);

    return () => controller.abort();
  }, [url]);

  return connection;
}

async function follow(
  url: string,
  signal: AbortSignal,
  handlers: { current: StreamHandlers },
  setConnection: (connection: EventsConnection) => void,
): Promise<void> {
  while (!signal.aborted) {
    // A reconnect is a subscription being established again, so the stream is not connected until
    // the server has said so a second time.
    setConnection('connecting');

    for await (const chunk of streamFrames({ url, signal })) {
      if (chunk.kind === 'frame') {
        apply(streamEvent(chunk.frame), handlers.current, setConnection);
        continue;
      }

      setConnection('delayed');
      handlers.current.onEnded?.();
      if (chunk.kind === 'refused' && chunk.status === UNAUTHORIZED) return;
    }

    if (signal.aborted) return;
    await rest(DECLARED_RETRY_MILLISECONDS);
  }
}

const UNAUTHORIZED = 401;

function apply(event: StreamEvent, handlers: StreamHandlers, setConnection: (state: EventsConnection) => void): void {
  if (event.kind === 'ready') {
    setConnection('connected');
    handlers.onReady();
    return;
  }

  if (event.kind === 'changed') handlers.onChanged({ resource: event.resource, id: event.id, version: event.version });
}

function rest(milliseconds: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, milliseconds);
  });
}
