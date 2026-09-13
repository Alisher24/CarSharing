import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { changedResources, coalesceSignals, recordSignal, signalKey, type Signal } from './changes.ts';

function change(id: string, version: string, resource: Signal['resource'] = 'vehicles'): Signal {
  return { resource, id, version };
}

describe('remembering the highest version of a resource', () => {
  test('a version seen for the first time is remembered', () => {
    const observed = recordSignal(new Map(), change('v1', '42'));

    assert.equal(observed.get(signalKey(change('v1', '7'))), '42');
  });

  test('a later, higher version replaces a lower one', () => {
    const observed = recordSignal(recordSignal(new Map(), change('v1', '42')), change('v1', '43'));

    assert.equal(observed.get(signalKey(change('v1', '7'))), '43');
  });

  // A repeated signal is allowed: the stream promises no exactly-once delivery, so the same change
  // can arrive twice and must not lower what is remembered.
  test('a repeated or older signal leaves the version where it was', () => {
    const once = recordSignal(new Map(), change('v1', '43'));
    const repeated = recordSignal(once, change('v1', '43'));
    const older = recordSignal(repeated, change('v1', '42'));

    assert.equal(older.get(signalKey(change('v1', '7'))), '43');
  });

  test('two resources with the same identifier are kept apart', () => {
    const observed = recordSignal(recordSignal(new Map(), change('same', '1')), change('same', '2', 'zones'));

    assert.deepEqual([...observed.values()].sort(), ['1', '2']);
  });
});

describe('coalescing one window of signals', () => {
  // A batch of changes to one resource is one request for it rather than one request per change,
  // and the request only has to publish the highest version the batch reached.
  test('a batch of changes to one resource keeps the highest version of it', () => {
    const window = coalesceSignals([change('v1', '42'), change('v1', '43'), change('v2', '7'), change('v1', '41')]);

    assert.equal(window.size, 2);
    assert.equal(window.get(signalKey(change('v1', '0'))), '43');
    assert.equal(window.get(signalKey(change('v2', '0'))), '7');
  });

  test('a window with no signal asks for nothing', () => {
    assert.equal(coalesceSignals([]).size, 0);
  });

  // The read a window asks for is per resource, so the fleet is read once however many of its
  // vehicles changed in the window.
  test('a window names each resource it changed once', () => {
    const window = [change('v1', '42'), change('v2', '7'), change('z1', '1', 'zones'), change('v1', '43')];

    assert.deepEqual(changedResources(window), ['vehicles', 'zones']);
    assert.deepEqual(changedResources([]), []);
  });
});
