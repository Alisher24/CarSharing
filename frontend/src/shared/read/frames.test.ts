import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { frameLines, readFrames } from './frames.ts';

/** The body of a response, split into the chunks a reader happens to receive. */
async function* chunks(...parts: readonly string[]): AsyncGenerator<Uint8Array> {
  const encoder = new TextEncoder();
  for (const part of parts) yield encoder.encode(part);
}

async function framesOf(...parts: readonly string[]) {
  const read = [];
  for await (const frame of readFrames(chunks(...parts))) read.push(frame);
  return read;
}

describe('reading the frames of a stream', () => {
  test('the handshake and a change are read as two frames', async () => {
    const frames = await framesOf(
      'retry: 3000\nevent: ready\ndata: {"server_time":"2026-09-12T07:15:30.123456Z"}\n\n' +
        'event: vehicle.changed\ndata: {"id":"v1","version":"42"}\n\n',
    );

    assert.deepEqual(frames, [
      { event: 'ready', data: '{"server_time":"2026-09-12T07:15:30.123456Z"}' },
      { event: 'vehicle.changed', data: '{"id":"v1","version":"42"}' },
    ]);
  });

  test('a frame split between two chunks is read once, whole', async () => {
    const frames = await framesOf('event: vehicle.changed\nda', 'ta: {"id":"v1","version":"42"}\n\n');

    assert.deepEqual(frames, [{ event: 'vehicle.changed', data: '{"id":"v1","version":"42"}' }]);
  });

  test('a frame that has not ended is not read yet', async () => {
    assert.deepEqual(await framesOf('event: vehicle.changed\ndata: {"id":"v1","version":"42"}\n'), []);
  });

  test('a comment carries no frame of its own', async () => {
    const frames = await framesOf(': keepalive\n\nevent: vehicle.changed\ndata: {"id":"v1","version":"42"}\n\n');

    assert.deepEqual(frames, [{ event: 'vehicle.changed', data: '{"id":"v1","version":"42"}' }]);
  });

  // One frame may be written across several chunks when a proxy re-splits the body, so the reading
  // cannot rely on a chunk carrying a whole frame.
  test('several frames in one chunk are all read', async () => {
    const frames = await framesOf(
      'event: zone.changed\ndata: {"id":"z1","version":"1"}\n\nevent: tariff.changed\ndata: {"id":"t1","version":"2"}\n\n',
    );

    assert.equal(frames.length, 2);
    assert.deepEqual(
      frames.map((frame) => frame.event),
      ['zone.changed', 'tariff.changed'],
    );
  });

  test('a field this contract does not declare is dropped rather than guessed at', () => {
    assert.deepEqual(frameLines('id: 7\nevent: ready\ndata: {"server_time":"now"}'), {
      event: 'ready',
      data: '{"server_time":"now"}',
    });
  });

  test('a frame that names no event is not a frame', () => {
    assert.equal(frameLines('data: {"id":"v1","version":"42"}'), undefined);
    assert.equal(frameLines(': keepalive'), undefined);
  });

  test('several data lines of one frame are one payload', () => {
    assert.deepEqual(frameLines('event: ready\ndata: one\ndata: two'), { event: 'ready', data: 'one\ntwo' });
  });
});
