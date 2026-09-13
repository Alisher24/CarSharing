import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { afterSuccess, type Resource } from '../../shared/api/Resource.ts';
import { collection, warningNotification } from './fixtures.ts';
import { shownCollection, updatesCollection, type NotificationReading } from './notificationReading.ts';

const OWNER = 'account-a';
const STRANGER = 'account-b';
const RENTAL = '01994342-6ba7-7000-8000-000300000001';
const WARNING = '01994342-6ba7-7000-8000-000400000001';

/** One answer of the collection, as the reader of one account produced it. */
function reading(session: string, serverTime: string, version = '1'): NotificationReading {
  return { session, collection: collection(serverTime, [warningNotification(WARNING, RENTAL, { version })]) };
}

function resource(value: NotificationReading | undefined): Resource<NotificationReading | undefined> {
  return afterSuccess(value);
}

describe('one answer of the notification collection', () => {
  test('is the first thing a reader holds, and covers nothing before it', () => {
    assert.equal(updatesCollection(undefined, collection('2026-09-13T07:14:30.000000Z', [])), true);
  });

  test('is stored when it was computed later than the one already held', () => {
    const held = collection('2026-09-13T07:14:30.000000Z', []);
    const later = collection('2026-09-13T07:14:31.000000Z', []);

    assert.equal(updatesCollection(held, later), true);
  });

  // A late answer from an older read describes a notification as it was before something the client
  // has already been told: storing it would show a warning that was read as unread again.
  test('does not replace a newer version of a notification it publishes', () => {
    const held = collection('2026-09-13T07:14:31.000000Z', [
      warningNotification(WARNING, RENTAL, { version: '2', readAt: '2026-09-13T07:14:30.500000Z' }),
    ]);
    const late = collection('2026-09-13T07:14:30.000000Z', [warningNotification(WARNING, RENTAL, { version: '1' })]);

    assert.equal(updatesCollection(held, late), false);
    assert.equal(updatesCollection(held, held), false, 'a repeated answer was stored as a newer one');
  });
});

describe('the collection the interface shows', () => {
  test('is the one read for the account on screen', () => {
    const shown = shownCollection(resource(reading(OWNER, '2026-09-13T07:14:30.000000Z')), OWNER);

    assert.equal(shown?.collection.items[0]?.id, WARNING);
    assert.ok(shown?.receivedAt instanceof Date);
  });

  // A switch builds a new reader, and the answer of the previous account is refused by the new one
  // whatever happens to the read that replaces it: a failure must not leave the old warning on screen.
  test('is nothing when the answer belongs to another account', () => {
    const answer = resource(reading(STRANGER, '2026-09-13T07:14:30.000000Z'));

    assert.equal(shownCollection(answer, OWNER), undefined);
  });

  test('is nothing when nobody is signed in, and nothing before an answer arrives', () => {
    assert.equal(shownCollection(resource(reading(OWNER, '2026-09-13T07:14:30.000000Z')), undefined), undefined);
    assert.equal(shownCollection({ phase: 'loading' }, OWNER), undefined);
    assert.equal(shownCollection({ phase: 'failed' }, OWNER), undefined);
  });

  // The last reading of the account on screen stays through a failure of the next one, which is the
  // same rule the reservation panel follows: a service that stopped answering does not empty a screen.
  test('is the last reading of that account when the next read failed', () => {
    const shown = shownCollection(
      { phase: 'stale', value: reading(OWNER, '2026-09-13T07:14:30.000000Z'), loadedAt: new Date() },
      OWNER,
    );

    assert.equal(shown?.collection.items.length, 1);
  });
});
