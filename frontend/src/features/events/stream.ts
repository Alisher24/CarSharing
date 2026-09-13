import { readFrames, type Frame } from './frames.ts';

/**
 * What one response to a subscription turned out to be. An ended stream is an answer rather than a
 * failure: the server closes a stream when a queue overflows or a session is revoked, and both are
 * repaired by connecting again and reading the snapshots.
 */
export type StreamChunk = { kind: 'frame'; frame: Frame } | { kind: 'ended' } | { kind: 'refused'; status: number };

/** What a subscription needs to open one response. */
export type StreamRequest = {
  url: string;
  signal: AbortSignal;

  /** How the connection is made, which a test replaces with its own transport. */
  send?: typeof fetch;
};

// The private stream is authorised by the session cookie, so a request for a stream carries one;
// the public stream sends it as well and is unaffected by it.
const STREAM_REQUEST = {
  headers: { Accept: 'text/event-stream' },
  credentials: 'same-origin',
  cache: 'no-store',
} as const;

// The stream is read with fetch and a body reader rather than with the generated SSE client. That
// client carries an event identifier back on every reconnect, and this contract declares no
// identifier and no replay: sending one would ask the server for something it does not have. The
// reader below states the reconnection contract this client actually has, and it is the part of
// the stream a test can hold.
export async function* streamFrames({ url, signal, send = fetch }: StreamRequest): AsyncGenerator<StreamChunk> {
  let response: Response;
  try {
    response = await send(url, { ...STREAM_REQUEST, signal });
  } catch {
    yield { kind: 'ended' };
    return;
  }

  if (!response.ok || response.body === null) {
    yield { kind: 'refused', status: response.status };
    return;
  }

  for await (const frame of readFrames(readChunks(response.body))) {
    yield { kind: 'frame', frame };
  }

  yield { kind: 'ended' };
}

async function* readChunks(body: ReadableStream<Uint8Array>): AsyncGenerator<Uint8Array> {
  const reader = body.getReader();
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) return;
      yield value;
    }
  } finally {
    void reader.cancel().catch(() => undefined);
  }
}
