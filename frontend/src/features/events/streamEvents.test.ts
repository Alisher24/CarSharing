import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { streamEvent } from './streamEvents.ts';
import type { Frame } from './frames.ts';

describe('reading what a frame says', () => {
  test('the handshake carries the moment the subscription was established', () => {
    const event = streamEvent({ event: 'ready', data: '{"server_time":"2026-09-12T07:15:30.123456Z"}' });

    assert.deepEqual(event, { kind: 'ready', serverTime: '2026-09-12T07:15:30.123456Z' });
  });

  test('each change event names the resource it is about', () => {
    const changes = [
      ['vehicle.changed', 'vehicles'],
      ['zone.changed', 'zones'],
      ['tariff.changed', 'tariffs'],
    ] as const;

    for (const [name, resource] of changes) {
      const event = streamEvent({ event: name, data: '{"id":"r1","version":"42"}' });
      assert.deepEqual(event, { kind: 'changed', resource, id: 'r1', version: '42' });
    }
  });

  test('a version is kept as the decimal string the contract states', () => {
    const event = streamEvent({ event: 'vehicle.changed', data: '{"id":"r1","version":"9223372036854775807"}' });

    assert.equal(event.kind === 'changed' ? event.version : undefined, '9223372036854775807');
  });

  // The stream is a signal rather than a log, so a frame this client cannot use is dropped and the
  // reconciliation reads the snapshots that matter instead.
  test('a frame this contract does not declare says nothing', () => {
    assert.deepEqual(streamEvent({ event: 'rental.changed', data: '{"id":"r1","version":"1"}' }), { kind: 'ignored' });
    assert.deepEqual(streamEvent({ event: 'ready' }), { kind: 'ignored' });
  });

  test('a payload that is not the shape the contract states says nothing', () => {
    for (const data of ['not json', '{"id":"r1"}', '{"version":"42"}', '{"id":7,"version":"42"}', 'null']) {
      const frame: Frame = { event: 'vehicle.changed', data };
      assert.deepEqual(streamEvent(frame), { kind: 'ignored' });
    }
  });
});
