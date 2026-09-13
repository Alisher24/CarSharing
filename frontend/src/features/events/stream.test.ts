import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { streamFrames, type StreamChunk } from './stream.ts';

/** A response body that delivers the given frames and then ends, as a connection would. */
function bodyOf(...frames: readonly string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const frame of frames) controller.enqueue(encoder.encode(frame));
      controller.close();
    },
  });
}

function respond(status: number, body?: ReadableStream<Uint8Array>): typeof fetch {
  return () => Promise.resolve(new Response(body ?? null, { status }));
}

async function read(send: typeof fetch): Promise<StreamChunk[]> {
  const chunks: StreamChunk[] = [];
  const controller = new AbortController();

  for await (const chunk of streamFrames({ url: '/api/v1/events', signal: controller.signal, send })) {
    chunks.push(chunk);
  }

  return chunks;
}

describe('reading one stream response', () => {
  test('the frames of a response are handed on, and the end of it is reported', async () => {
    const send = respond(
      200,
      bodyOf(
        'retry: 3000\nevent: ready\ndata: {"server_time":"2026-09-12T07:15:30.123456Z"}\n\n',
        'event: vehicle.changed\ndata: {"id":"v1","version":"42"}\n\n',
      ),
    );

    assert.deepEqual(await read(send), [
      { kind: 'frame', frame: { event: 'ready', data: '{"server_time":"2026-09-12T07:15:30.123456Z"}' } },
      { kind: 'frame', frame: { event: 'vehicle.changed', data: '{"id":"v1","version":"42"}' } },
      { kind: 'ended' },
    ]);
  });

  // The server closes a stream whose queue overflowed or whose session was revoked, and the client
  // repairs that by connecting again and reading the snapshots rather than by asking for a replay.
  test('a response that ends without a frame is reported as an end', async () => {
    assert.deepEqual(await read(respond(200, bodyOf())), [{ kind: 'ended' }]);
  });

  test('a refusal is reported with the status the server answered', async () => {
    assert.deepEqual(await read(respond(401)), [{ kind: 'refused', status: 401 }]);
  });

  test('a connection that could not be made is reported as an end', async () => {
    const send: typeof fetch = () => Promise.reject(new Error('offline'));

    assert.deepEqual(await read(send), [{ kind: 'ended' }]);
  });

  test('a stream is requested as an event stream and never from a cache', async () => {
    let requested: RequestInit | undefined;
    const send: typeof fetch = (_url, init) => {
      requested = init;
      return Promise.resolve(new Response(bodyOf(), { status: 200 }));
    };

    await read(send);

    assert.equal(new Headers(requested?.headers).get('Accept'), 'text/event-stream');
    assert.equal(requested?.cache, 'no-store');
    assert.equal(requested?.credentials, 'same-origin', 'the private stream is authorised by cookie');
  });

  test('a body that arrives in pieces is read whole', async () => {
    const encoder = new TextEncoder();
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode('event: vehicle.chang'));
        controller.enqueue(encoder.encode('ed\ndata: {"id":"v1","version":"42"}\n'));
        controller.enqueue(encoder.encode('\n'));
        controller.close();
      },
    });

    assert.deepEqual(await read(respond(200, body)), [
      { kind: 'frame', frame: { event: 'vehicle.changed', data: '{"id":"v1","version":"42"}' } },
      { kind: 'ended' },
    ]);
  });
});
