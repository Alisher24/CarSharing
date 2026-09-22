import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { afterFailure, afterSuccess, loadedValue, type Resource } from './Resource.ts';

const loadedAt = new Date('2026-09-13T07:15:30.000Z');

describe('what a resource keeps when loading it fails', () => {
  test('a first failure has nothing to show and says so', () => {
    const failed = afterFailure<string[]>({ phase: 'loading' });
    assert.equal(failed.phase, 'failed');
    assert.equal(loadedValue(failed), undefined);
  });

  test('a failure after a reading keeps that reading and marks it stale', () => {
    const stale = afterFailure<string[]>({ phase: 'ready', value: ['первый'], loadedAt });
    assert.equal(stale.phase, 'stale');
    assert.deepEqual(loadedValue(stale), ['первый']);
    assert.equal(stale.phase === 'stale' ? stale.loadedAt : undefined, loadedAt);
  });

  test('a further failure keeps the moment of the last success rather than the last attempt', () => {
    const twiceFailed = afterFailure(afterFailure<string[]>({ phase: 'ready', value: ['первый'], loadedAt }));
    assert.equal(twiceFailed.phase === 'stale' ? twiceFailed.loadedAt : undefined, loadedAt);
  });

  test('a success clears the mark and replaces the reading', () => {
    const stale: Resource<string[]> = { phase: 'stale', value: ['первый'], loadedAt };
    const recovered = afterSuccess(['второй']);
    assert.equal(recovered.phase, 'ready');
    assert.deepEqual(loadedValue(recovered), ['второй']);
    assert.notDeepEqual(loadedValue(recovered), loadedValue(stale));
  });

  // An empty answer is a reading like any other: the service said there is nothing, which is not
  // the same as never having been asked.
  test('an empty successful reading is held as a reading', () => {
    const empty = afterSuccess<string[]>([]);
    assert.equal(empty.phase, 'ready');
    assert.deepEqual(loadedValue(empty), []);
    assert.equal(afterFailure(empty).phase, 'stale');
  });
});
