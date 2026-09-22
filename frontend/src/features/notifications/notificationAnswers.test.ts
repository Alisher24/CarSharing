import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { changedResources, type Signal } from '../../shared/read/changes.ts';
import { createReadCoordinator, type ReadCoordinator } from '../../shared/read/coordinator.ts';
import { collection, warningNotification } from '../../shared/account/fixtures.ts';
import { notificationAnswers, type NotificationSession } from './notificationAnswers.ts';
import type { NotificationReading } from './notificationReading.ts';

const OWNER = 'account-a';
const RENTAL = '01994342-6ba7-7000-8000-000300000001';
const WARNING = '01994342-6ba7-7000-8000-000400000001';

/** The reader of one account: its coordinator together with the state its answers are given. */
function readerOf(session: string): { state: NotificationSession; read: ReadCoordinator } {
  const state: NotificationSession = { session, reading: { collection: undefined } };
  return { state, read: createReadCoordinator(session, notificationAnswers(state)) };
}

/** One answer of the collection as the reader of the owner produced it. */
function reading(version: string, serverTime = '2026-09-13T07:14:30.000000Z'): NotificationReading {
  return { session: OWNER, collection: collection(serverTime, [warningNotification(WARNING, RENTAL, { version })]) };
}

function signal(version: string): Signal {
  return { resource: 'notifications', id: WARNING, version };
}

/** One complete read of the collection: opened, answered, stored and closed the way the hook does it. */
function readCollection(state: NotificationSession, read: ReadCoordinator, answer: NotificationReading): void {
  const ticket = read.begin('notifications');
  assert.notEqual(ticket, undefined);

  const stored = notificationAnswers(state).notifications.observe(answer, answer.session);
  read.cover('notifications', stored?.covered ?? new Map());
  read.stored(ticket!);
  read.settle('notifications');
}

describe('one signal of the private stream', () => {
  test('asks for the collection to be read, and the answer covers the version it named', () => {
    const { state, read } = readerOf(OWNER);
    read.observe('notifications', WARNING, '1');
    read.request(['notifications']);
    assert.equal(read.due('notifications'), true, 'a signalled change asked for nothing');

    const ticket = read.begin('notifications');
    assert.notEqual(ticket, undefined);
    const answer = reading('1');
    const stored = notificationAnswers(state).notifications.observe(answer, OWNER);

    assert.equal(stored?.value, answer, 'the answer was not stored as the reading it is');
    assert.equal(stored?.covered.get(WARNING), '1');

    read.cover('notifications', stored?.covered ?? new Map());
    read.stored(ticket!);
    read.settle('notifications');

    assert.equal(read.due('notifications'), false, 'an answered change asked for a second read');
  });

  // The stream is a signal rather than a log: a frame that arrives twice describes one change, so it
  // costs one read and produces one entry rather than two.
  test('is one read when the same frame arrives twice', () => {
    const { state, read } = readerOf(OWNER);
    const frames = [signal('1'), signal('1')];
    assert.deepEqual(changedResources(frames), ['notifications']);

    for (const frame of frames) read.observe(frame.resource, frame.id, frame.version);
    readCollection(state, read, reading('1'));

    assert.equal(read.due('notifications'), false, 'the frame that arrived twice asked for another read');
    assert.equal(state.reading.collection?.items.length, 1, 'the warning was stored twice');
  });

  test('is read again when a newer version arrives while the older answer is held', () => {
    const { state, read } = readerOf(OWNER);
    readCollection(state, read, reading('1'));

    read.observe('notifications', WARNING, '2');
    read.request(['notifications']);

    assert.equal(read.due('notifications'), true, 'a newer version was treated as covered');
    assert.equal(state.reading.collection?.items.length, 1);
  });
});

describe('an answer of the collection', () => {
  test('is refused when it belongs to another account', () => {
    const { state } = readerOf(OWNER);
    const answers = notificationAnswers(state);
    const stranger: NotificationReading = {
      session: 'account-b',
      collection: collection('2026-09-13T07:14:30.000000Z', []),
    };

    assert.equal(answers.notifications.accepts(stranger.session), false);
    assert.equal(answers.notifications.observe(stranger, stranger.session), undefined);
    assert.equal(state.reading.collection, undefined, "another account's answer was stored");
  });

  test('stores nothing when the reader carried no collection', () => {
    const { state } = readerOf(OWNER);

    assert.equal(notificationAnswers(state).notifications.observe(undefined, OWNER), undefined);
    assert.equal(state.reading.collection, undefined);
  });

  test('does not let a late answer replace a newer version of a warning', () => {
    const { state } = readerOf(OWNER);
    const answers = notificationAnswers(state);
    answers.notifications.observe(reading('2', '2026-09-13T07:14:31.000000Z'), OWNER);

    assert.equal(answers.notifications.observe(reading('1'), OWNER), undefined);
    assert.equal(state.reading.collection?.items[0]?.version, '2', 'the held version rolled back');
  });

  test('is answered by no other document of this reader', () => {
    const { state } = readerOf(OWNER);
    const answers = notificationAnswers(state);

    for (const document of ['vehicles', 'zones', 'tariffs', 'rentals', 'invoices'] as const) {
      assert.equal(answers[document].accepts(OWNER), false, `${document} was accepted by the notification reader`);
      assert.equal(answers[document].observe({}, OWNER), undefined, `${document} was stored by it`);
    }
  });
});
