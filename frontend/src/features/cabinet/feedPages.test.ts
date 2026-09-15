import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import {
  continuationCursor,
  emptyReading,
  mergedReading,
  type FeedPage,
  type FeedReading,
  type Replaces,
} from './feedPages.ts';

/** One record of a feed in these checks: an identifier and a reading of it that can move. */
type Record = { id: string; version: string };

/** A record that never changes, which is what a finished ride is. */
const NEVER_REPLACED: Replaces<Record> = () => false;

/** A record replaced by a reading newer than the one on screen, which is what an invoice is. */
const NEWER_VERSION: Replaces<Record> = (held, answered) => Number(answered.version) > Number(held.version);

function record(id: string, version = '1'): Record {
  return { id, version };
}

function newest(records: readonly Record[], nextCursor: string | null = null): FeedPage<Record> {
  return { records, nextCursor, placement: 'newest' };
}

function continuation(records: readonly Record[], nextCursor: string | null = null): FeedPage<Record> {
  return { records, nextCursor, placement: 'continuation' };
}

function idsOf(reading: FeedReading<Record>): string[] {
  return reading.records.map((one) => one.id);
}

describe('reading one page into a feed', () => {
  test('the first page is the feed', () => {
    const read = mergedReading(emptyReading<Record>(), newest([record('c'), record('b')]), NEVER_REPLACED);

    assert.deepEqual(idsOf(read), ['c', 'b']);
  });

  test('a continuation is added below what is on screen', () => {
    const first = mergedReading(emptyReading<Record>(), newest([record('c')], 'cursor-1'), NEVER_REPLACED);
    const second = mergedReading(first, continuation([record('b'), record('a')]), NEVER_REPLACED);

    assert.deepEqual(idsOf(second), ['c', 'b', 'a']);
  });

  test('a record the newest page names for the first time is added above what is on screen', () => {
    const held = mergedReading(emptyReading<Record>(), newest([record('b'), record('a')]), NEVER_REPLACED);
    const again = mergedReading(held, newest([record('c'), record('b'), record('a')]), NEVER_REPLACED);

    assert.deepEqual(idsOf(again), ['c', 'b', 'a']);
  });

  // The whole point of merging rather than replacing: the record the newest page no longer names is
  // behind a cursor that was taken from it, so replacing the page would lose it altogether.
  test('a record the newest page no longer reaches stays on screen', () => {
    const held = mergedReading(emptyReading<Record>(), newest([record('b'), record('a')], 'cursor-1'), NEVER_REPLACED);
    const again = mergedReading(held, newest([record('d'), record('c')], 'cursor-2'), NEVER_REPLACED);

    assert.deepEqual(idsOf(again), ['d', 'c', 'b', 'a']);
  });
});

describe('what a record that came back again does to the one on screen', () => {
  test('a newer reading replaces it where it stands', () => {
    const held = mergedReading(emptyReading<Record>(), newest([record('b'), record('a')]), NEWER_VERSION);
    const again = mergedReading(held, newest([record('b', '2')]), NEWER_VERSION);

    assert.deepEqual(idsOf(again), ['b', 'a'], 'the record moved');
    assert.equal(again.records[0]?.version, '2');
  });

  test('a reading older than the one on screen is refused', () => {
    const held = mergedReading(emptyReading<Record>(), newest([record('b', '3')]), NEWER_VERSION);
    const again = mergedReading(held, newest([record('b', '2')]), NEWER_VERSION);

    assert.equal(again.records[0]?.version, '3');
  });

  test('a record that never changes keeps the reading on screen', () => {
    const held = mergedReading(emptyReading<Record>(), newest([record('b', '1')]), NEVER_REPLACED);
    const again = mergedReading(held, newest([record('b', '9')]), NEVER_REPLACED);

    assert.equal(again.records[0]?.version, '1');
  });
});

describe('where the page after the feed starts', () => {
  test('nothing read yet continues from nowhere, so the newest page is read', () => {
    assert.equal(continuationCursor(emptyReading<Record>().continuation), undefined);
  });

  test('the first page states where the one after it starts', () => {
    const read = mergedReading(emptyReading<Record>(), newest([record('c')], 'cursor-1'), NEVER_REPLACED);

    assert.equal(continuationCursor(read.continuation), 'cursor-1');
  });

  test('a continuation moves it on', () => {
    const first = mergedReading(emptyReading<Record>(), newest([record('c')], 'cursor-1'), NEVER_REPLACED);
    const second = mergedReading(first, continuation([record('b')], 'cursor-2'), NEVER_REPLACED);

    assert.equal(continuationCursor(second.continuation), 'cursor-2');
  });

  test('a page that ends the feed leaves nothing to continue from', () => {
    const first = mergedReading(emptyReading<Record>(), newest([record('c')], 'cursor-1'), NEVER_REPLACED);
    const second = mergedReading(first, continuation([record('b')], null), NEVER_REPLACED);

    assert.equal(continuationCursor(second.continuation), undefined);
  });

  // The cursor of the newest page names the second page, which the feed is already past: taking it
  // would read a page a person has already scrolled through instead of the one after them.
  test('re-reading the newest page leaves it where the continuation put it', () => {
    const first = mergedReading(emptyReading<Record>(), newest([record('c')], 'cursor-1'), NEVER_REPLACED);
    const second = mergedReading(first, continuation([record('b')], 'cursor-2'), NEVER_REPLACED);
    const again = mergedReading(second, newest([record('d'), record('c')], 'cursor-3'), NEVER_REPLACED);

    assert.equal(continuationCursor(again.continuation), 'cursor-2');
  });
});
