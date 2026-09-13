// The notification operations, observed on the real HTTP boundary and in the PostgreSQL the running
// service reads: the owner's own collection, the keyset page a cursor continues, every refusal a
// cursor that belongs elsewhere meets, and the read that happens once however often it is repeated.
//
// The collection of a known shape is written directly, because a check cannot wait for twenty
// warnings. The warning a check marks read is one this build produced: the deadline of a real
// reservation is moved into its last minute with the worker stopped, so the collection is what
// creates it.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import { call, compose, foreignOrigin, waitForReady } from './client.mjs';
import {
  endSuiteNotifications,
  firstCursorOf,
  insertNotification,
  issueCursor,
  markRead,
  newSuiteAccount,
  notificationsOf,
  NOTIFICATIONS_PATH,
  ownerOf,
  positionOf,
  sql,
  storedNotification,
  storedStage,
  tamper,
} from './notifications.mjs';
import { availableVehicle, endSuiteReservations, moveDeadline, newCommandKey, reserve } from './reservations.mjs';

const MOMENT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;

/**
 * The moment the notifications of this check are measured from, read once from the database clock as
 * the literal a query can state it as. It is read once because the collection is ordered by the
 * moment a notification was created, and two rows written one after another would otherwise differ
 * by milliseconds in the opposite direction to the order the check meant to write them in.
 */
let referenceMoment = '';

/**
 * storedAgo writes one notification created a stated number of seconds before the moment above, so a
 * check that writes several knows the order the collection publishes them in: the smallest number of
 * seconds is the newest.
 */
function storedAgo(account, seconds) {
  return insertNotification({
    account,
    at: `'${referenceMoment}'::timestamptz - make_interval(secs => ${seconds})`,
  });
}

/**
 * publishedOrder is the order the collection publishes one account's notifications in, read from the
 * database: newest first, with the identifier as the tie-break. A check compares the answer with it
 * rather than with the order it wrote the rows in, because two rows written one after another differ
 * by milliseconds in a direction the check does not control.
 */
function publishedOrder(account) {
  const rows = sql(
    `SELECT string_agg(note.id::text, ',' ORDER BY note.created_at DESC, note.id DESC)
     FROM notifications note
     JOIN users account ON account.id = note.user_id
     WHERE account.email = '${account.email}'`,
  );
  return rows === '' ? [] : rows.split(',');
}

/** The alphabet the contract declares for a cursor, which a page link has to carry unescaped. */
const CURSOR_ALPHABET = /^[A-Za-z0-9_-]+$/;

before(async () => {
  await waitForReady();
  // Read once and stated as a literal, so every notification of this suite is measured from the same
  // moment rather than from the moment its own statement happened to run.
  referenceMoment = sql('SELECT clock_timestamp()');
});

// A check that left a reservation behind would hold a vehicle the next one needs, and the suites
// after this one read the prepared demonstration.
beforeEach(endSuiteNotifications);

after(endSuiteNotifications);

describe('reading the collection of one account', () => {
  test('answers an empty collection with a null cursor and the moment of the read', async () => {
    const account = await newSuiteAccount('empty');

    const answer = await notificationsOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.deepEqual(answer.json.items, []);
    assert.equal(answer.json.next_cursor, null);
    assert.match(answer.json.server_time, MOMENT);

    // The owner comes from the session, so a fresh account reads nothing even though the database
    // holds notifications for other accounts.
    assert.ok(
      Number(sql('SELECT count(*) FROM notifications')) > 0,
      'the check proved nothing: the database holds no notification at all',
    );
  });

  test('answers the owner own notifications newest first and nobody else', async () => {
    const account = await newSuiteAccount('own');
    const stranger = await newSuiteAccount('stranger');
    const older = storedAgo(account, 300);
    const newer = storedAgo(account, 100);
    const foreign = storedAgo(stranger, 200);

    const answer = await notificationsOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.deepEqual(
      answer.json.items.map((one) => one.id),
      [newer.id, older.id],
    );
    const [first] = answer.json.items;
    assert.equal(first.type, 'reservation_expiring');
    assert.equal(first.active, true);
    assert.equal(first.version, '1');
    assert.equal(first.rental_id, newer.rentalId);
    assert.match(first.created_at, MOMENT);
    assert.match(first.expires_at, MOMENT);
    assert.equal(first.read_at, undefined);

    const theirs = await notificationsOf(stranger);
    assert.deepEqual(
      theirs.json.items.map((one) => one.id),
      [foreign.id],
      'another account read a notification that is not its own',
    );
  });

  test('leaves another account notifications out of the pages rather than shifting them', async () => {
    const account = await newSuiteAccount('paged-owner');
    const stranger = await newSuiteAccount('paged-stranger');
    const mine = [1, 2, 3, 4].map((seconds) => storedAgo(account, seconds * 100));
    for (const seconds of [150, 250, 350]) {
      storedAgo(stranger, seconds);
    }
    const published = publishedOrder(account);

    const first = await notificationsOf(account, '?limit=3');
    assert.equal(first.status, 200, first.text);
    assert.deepEqual(
      first.json.items.map((one) => one.id),
      published.slice(0, 3),
    );
    assert.match(first.json.next_cursor, CURSOR_ALPHABET);

    const second = await notificationsOf(account, cursorQuery(3, first.json.next_cursor));
    assert.equal(second.status, 200, second.text);
    assert.deepEqual(
      second.json.items.map((one) => one.id),
      published.slice(3),
      'the page after the cursor did not continue exactly after the last item',
    );
    assert.equal(second.json.next_cursor, null, 'the last page offered a cursor');

    const whole = await notificationsOf(account, '?limit=100');
    assert.equal(whole.json.items.length, 4);
    assert.equal(whole.json.next_cursor, null, 'a page holding the whole collection offered a cursor');
  });

  test('answers a full page in the middle with a cursor and repeats no item', async () => {
    const account = await newSuiteAccount('no-repeat');
    const stored = [1, 2, 3].map((seconds) => storedAgo(account, seconds * 100));
    assert.equal(stored.length, 3);
    const published = publishedOrder(account);

    const first = await notificationsOf(account, '?limit=2');
    const second = await notificationsOf(account, cursorQuery(2, first.json.next_cursor));
    const seen = [...first.json.items, ...second.json.items].map((one) => one.id);
    assert.deepEqual(seen, published);
    assert.equal(new Set(seen).size, seen.length, 'a cursor repeated an item it had already published');
  });

  test('defaults to the page size the contract declares', async () => {
    const account = await newSuiteAccount('default-page');
    for (let index = 0; index < 21; index += 1) {
      storedAgo(account, (index + 1) * 10);
    }

    const answer = await notificationsOf(account);
    assert.equal(answer.json.items.length, 20, 'the declared default page size was not applied');
    assert.match(answer.json.next_cursor, CURSOR_ALPHABET);
  });
});

describe('a cursor that belongs somewhere else', () => {
  test('refuses a cursor of another operation', async () => {
    const account = await newSuiteAccount('foreign-op');
    storedAgo(account, 100);
    storedAgo(account, 200);
    const issued = await firstCursorOf(account);
    const other = issueCursor(
      { operation: 'getRides', owner: await ownerOf(account), params: { limit: '1' } },
      positionOf(issued),
    );

    const refused = await notificationsOf(account, cursorQuery(1, other));
    assert.equal(refused.status, 400, refused.text);
    assert.equal(refused.json.code, 'INVALID_CURSOR');
  });

  test("refuses another account's cursor even when its own collection is not empty", async () => {
    const owner = await newSuiteAccount('cursor-owner');
    const stranger = await newSuiteAccount('cursor-stranger');
    storedAgo(owner, 100);
    storedAgo(owner, 200);
    storedAgo(stranger, 100);
    const issued = await firstCursorOf(owner);

    const refused = await notificationsOf(stranger, cursorQuery(1, issued));
    assert.equal(refused.status, 400, refused.text);
    assert.equal(refused.json.code, 'INVALID_CURSOR');
  });

  test('refuses a cursor whose payload was edited after it was signed', async () => {
    const account = await newSuiteAccount('tampered');
    storedAgo(account, 100);
    storedAgo(account, 200);
    const issued = await firstCursorOf(account);

    const refused = await notificationsOf(account, cursorQuery(1, tamper(issued)));
    assert.equal(refused.status, 400, refused.text);
    assert.equal(refused.json.code, 'INVALID_CURSOR');
  });

  test('refuses a cursor presented with another page size', async () => {
    const account = await newSuiteAccount('other-limit');
    storedAgo(account, 100);
    storedAgo(account, 200);
    const issued = await firstCursorOf(account);

    const refused = await notificationsOf(account, cursorQuery(5, issued));
    assert.equal(refused.status, 400, refused.text);
    assert.equal(refused.json.code, 'INVALID_CURSOR');
  });

  test('refuses a value that is not a cursor at all', async () => {
    const account = await newSuiteAccount('not-a-cursor');

    const malformed = await notificationsOf(account, '?cursor=not-a-cursor-at-all');
    assert.equal(malformed.status, 400, malformed.text);
    assert.equal(malformed.json.code, 'INVALID_CURSOR');
  });
});

describe('marking one notification read', () => {
  test('answers the final notification and repeats identically without another version', async () => {
    const account = await newSuiteAccount('mark-read');
    const { id } = storedAgo(account, 100);

    const answer = await markRead(id, account);
    assert.equal(answer.status, 200, answer.text);
    assert.equal(answer.json.notification.id, id);
    assert.equal(answer.json.notification.type, 'reservation_expiring');
    assert.equal(answer.json.notification.version, '2');
    assert.equal(answer.json.notification.active, true, 'reading deactivated the notification');
    assert.match(answer.json.notification.read_at, MOMENT);
    assert.match(answer.json.server_time, MOMENT);

    const stored = storedNotification(id);
    assert.equal(stored.version, 2);
    assert.equal(stored.active, true);
    assert.notEqual(stored.readAt, '');

    // The moment and the version are written once, so the repeat answers the representation the
    // first attempt stored rather than moving either of them.
    const repeat = await markRead(id, account);
    assert.equal(repeat.status, 200, repeat.text);
    assert.deepEqual(repeat.json.notification, answer.json.notification);
    assert.equal(storedNotification(id).version, 2, 'a repeated read moved the version');
    assert.equal(storedNotification(id).readAt, stored.readAt, 'a repeated read moved the moment');
  });

  test('records the personal signal in the same transaction as the read', async () => {
    const account = await newSuiteAccount('mark-signal');
    const { id } = storedAgo(account, 100);

    // The worker delivers a recorded signal and completes it, so the check that observes the signal
    // of the read itself runs with the worker stopped.
    compose('stop', 'worker');
    try {
      sql(`DELETE FROM outbox WHERE resource_id = '${id}'`);

      const answer = await markRead(id, account);
      assert.equal(answer.status, 200, answer.text);

      const signal = sql(
        `SELECT recipient_id || '|' || version || '|' || coalesce(completed_at::text, '') ` +
          `FROM outbox WHERE resource_id = '${id}' AND kind = 'notification.changed'`,
      );
      const [recipient, version, completedAt] = signal.split('|');
      assert.equal(recipient, await ownerOf(account), 'the signal was not addressed to the owner');
      assert.equal(Number(version), 2);
      assert.equal(completedAt, '', 'the signal of the read was already delivered');

      // The repeat publishes nothing: a version that did not move is not a change to announce.
      const repeated = await markRead(id, account);
      assert.equal(repeated.status, 200, repeated.text);
      assert.equal(
        Number(sql(`SELECT count(*) FROM outbox WHERE resource_id = '${id}'`)),
        1,
        'a repeated read recorded a second signal',
      );
    } finally {
      compose('start', 'worker');
    }
  });

  test('refuses a foreign and an unknown identifier indistinguishably', async () => {
    const owner = await newSuiteAccount('read-owner');
    const stranger = await newSuiteAccount('read-stranger');
    const { id } = storedAgo(owner, 100);

    const foreign = await markRead(id, stranger);
    const unknown = await markRead('01994342-6ba7-7000-8000-000000000001', stranger);
    assert.equal(foreign.status, 404, foreign.text);
    assert.equal(unknown.status, 404, unknown.text);
    assert.equal(foreign.json.code, 'RESOURCE_NOT_FOUND');
    assert.equal(foreign.json.code, unknown.json.code);
    assert.equal(storedNotification(id).version, 1, 'a foreign read changed the notification');
    assert.equal(storedNotification(id).readAt, '');
  });

  test('refuses a request without a session, without the token and from a foreign origin', async () => {
    const account = await newSuiteAccount('read-refusals');
    const { id } = storedAgo(account, 100);

    const anonymous = await call(`${NOTIFICATIONS_PATH}/${id}/read`, {
      method: 'POST',
      csrfToken: account.csrfToken,
    });
    assert.equal(anonymous.status, 401, anonymous.text);
    assert.equal(anonymous.json.code, 'AUTHENTICATION_REQUIRED');

    const untokened = await call(`${NOTIFICATIONS_PATH}/${id}/read`, {
      method: 'POST',
      cookie: account.cookie,
    });
    assert.equal(untokened.status, 403, untokened.text);
    assert.equal(untokened.json.code, 'CSRF_INVALID');

    const foreign = await markRead(id, account, { origin: foreignOrigin });
    assert.equal(foreign.status, 403, foreign.text);
    assert.equal(foreign.json.code, 'ORIGIN_NOT_ALLOWED');

    assert.equal(storedNotification(id).version, 1, 'a refused read changed the notification');
  });

  test('answers two racing reads identically and records one signal', async () => {
    const account = await newSuiteAccount('read-race');
    const { id } = storedAgo(account, 100);

    const [first, second] = await Promise.all([markRead(id, account), markRead(id, account)]);
    assert.equal(first.status, 200, first.text);
    assert.equal(second.status, 200, second.text);
    assert.deepEqual(first.json.notification, second.json.notification);
    assert.equal(storedNotification(id).version, 2, 'two racing reads moved the version twice');
    assert.equal(
      Number(sql(`SELECT count(*) FROM outbox WHERE resource_id = '${id}'`)),
      1,
      'two racing reads recorded two signals',
    );
  });
});

describe('the collection creates the warning the worker has not', () => {
  test('fixes a due warning for the reservation the reader holds, once', async () => {
    compose('stop', 'worker');
    try {
      const account = await newSuiteAccount('collection-warning');
      const created = await reserve(await availableVehicle(), newCommandKey(), account);
      assert.equal(created.status, 201, created.text);
      const rentalId = created.json.rental.id;
      moveDeadline(rentalId, 30);

      assert.equal(
        sql(`SELECT count(*) FROM notifications WHERE rental_id = '${rentalId}'`),
        '0',
        'a warning was created while no worker ran',
      );

      const answer = await notificationsOf(account);
      assert.equal(answer.status, 200, answer.text);
      const [warning] = answer.json.items;
      assert.equal(answer.json.items.length, 1, 'the collection did not fix the due warning');
      assert.equal(warning.type, 'reservation_expiring');
      assert.equal(warning.rental_id, rentalId);
      assert.equal(warning.active, true);
      assert.equal(warning.version, '1');

      // The signal is undelivered because no worker is running to deliver it, which is what makes
      // it the record of the collection's own transaction.
      assert.equal(
        sql(`SELECT count(*) FROM outbox WHERE resource_id = '${warning.id}' AND completed_at IS NULL`),
        '1',
        'the collection recorded no signal for the warning it created',
      );

      // A repeated read meets a warning that is already stored: it creates no second one and leaves
      // the moment and the version the first one stored alone.
      const again = await notificationsOf(account);
      assert.deepEqual(
        again.json.items.map((one) => one.id),
        [warning.id],
        'a repeated read created a second warning',
      );
      assert.equal(storedNotification(warning.id).version, 1, 'a repeated read moved the version');
      assert.equal(storedStage(rentalId), 'reserved', 'reading the collection released the reservation');
    } finally {
      compose('start', 'worker');
    }
    await endSuiteReservations();
  });

  test('creates nothing for a reservation that has more than its last minute left', async () => {
    const account = await newSuiteAccount('collection-quiet');
    const created = await reserve(await availableVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);

    const answer = await notificationsOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.deepEqual(answer.json.items, [], 'a warning was created outside the last minute');

    await endSuiteReservations();
  });

  test('creates nothing for a reservation whose deadline has passed, and releases it', async () => {
    compose('stop', 'worker');
    try {
      const account = await newSuiteAccount('collection-past');
      const created = await reserve(await availableVehicle(), newCommandKey(), account);
      assert.equal(created.status, 201, created.text);
      const rentalId = created.json.rental.id;
      moveDeadline(rentalId, -1);

      const answer = await notificationsOf(account);
      assert.equal(answer.status, 200, answer.text);
      assert.deepEqual(answer.json.items, [], 'a belated warning was created by the collection');
      // The collection fixed the expiry it met, which is what makes the vehicle free without the
      // worker; it did not warn a reservation that is no longer one.
      assert.equal(storedStage(rentalId), 'expired', 'the collection did not release the reservation');
    } finally {
      compose('start', 'worker');
    }
    await endSuiteReservations();
  });

  test('hands the second page of one collection a cursor that continues it', async () => {
    const account = await newSuiteAccount('collection-page');
    const stored = [1, 2, 3].map((seconds) => storedAgo(account, seconds * 100));

    const first = await notificationsOf(account, '?limit=1');
    const second = await notificationsOf(account, cursorQuery(1, first.json.next_cursor));
    assert.equal(second.status, 200, second.text);
    assert.deepEqual(
      second.json.items.map((one) => one.id),
      [stored[1].id],
      'the page after the cursor did not continue after the item the cursor named',
    );
    assert.match(second.json.server_time, MOMENT);
  });
});

/** The query one page of a stated size, continued from a stated cursor. */
function cursorQuery(limit, cursor) {
  return `?limit=${limit}&cursor=${encodeURIComponent(cursor)}`;
}
