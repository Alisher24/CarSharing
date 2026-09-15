// What the notification suites need beyond one HTTP call: the two private paths, the cursor the
// collection hands back, and the stored rows the checks read to prove what a page and a read
// actually did. A check that must reach a collection of a known shape writes its notifications
// directly, because the deadline that decides a warning is placed relative to the clock of the
// database rather than waited for.
import assert from 'node:assert/strict';
import { call } from './client.mjs';
import { CURSOR_ALPHABET } from './cursors.mjs';
import { insertRentalReturning } from './rentalrows.mjs';
import { endSuiteReservations, newAccount, sql } from './reservations.mjs';

export const NOTIFICATIONS_PATH = '/api/v1/me/notifications';

/** Every account this suite registers carries this prefix, which is also how its rows are found. */
const ACCOUNT_PREFIX = 'notifications';

/** One page of the caller's own notifications, as the request states it. */
export function notificationsOf(account, query = '') {
  return call(`${NOTIFICATIONS_PATH}${query}`, { cookie: account.cookie });
}

/** Marks one notification of the caller as read. */
export function markRead(notificationId, account, options = {}) {
  return call(`${NOTIFICATIONS_PATH}/${notificationId}/read`, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    ...options,
  });
}

/**
 * insertNotification writes one stored notification with the moments a check states, and returns its
 * identifier together with the rental it belongs to. The rental is written too, because a
 * notification about no rental is a row the module itself could never write. Both identifiers are
 * drawn by the database, so a written row carries the version the contract declares for a stored
 * resource.
 *
 * `at` is a SQL expression for the moment the notification was created, which defaults to the clock
 * of the database less the stated age. A check that writes several notifications states one moment
 * to measure them from, rather than leaving each row a few milliseconds later than the last: the
 * collection is ordered by that moment, and a check must know which order it wrote.
 */
export function insertNotification({ account, createdSecondsAgo, at, active = true, readAt = null }) {
  const createdAt = at ?? `clock_timestamp() - make_interval(secs => ${createdSecondsAgo})`;
  // Both identifiers are drawn by the database, which is also what the application reads them from,
  // so a written row carries the version the contract declares for a stored resource.
  const rentalId = sql(
    insertRentalReturning(
      {
        id: sql('SELECT uuidv7()'),
        email: account.email,
        vehicleId: anyVehicle(),
        stage: 'expired',
        reservedAt: `(${createdAt}) - interval '15 minutes'`,
        expiresAt: `(${createdAt}) + interval '1 second'`,
        endedAt: `(${createdAt}) + interval '1 second'`,
      },
      'id',
    ),
  );
  const id = sql(
    `WITH written AS (
       INSERT INTO notifications (id, user_id, rental_id, kind, created_at, read_at, active, version)
       SELECT uuidv7(), account.id, '${rentalId}', 'reservation_expiring', ${createdAt}, ` +
      `${readAt ?? 'null'}, ${active}, 1 ` +
      `FROM users account WHERE account.email = '${account.email}' ` +
      `RETURNING id
     )
     SELECT id FROM written`,
  );
  return { id, rentalId };
}

/** What the database holds for one notification: its version, its activity and its read moment. */
export function storedNotification(notificationId) {
  const row = sql(
    `SELECT version || '|' || active || '|' || coalesce(read_at::text, '') ` +
      `FROM notifications WHERE id = '${notificationId}'`,
  );
  const [version, active, readAt] = row.split('|');
  return { version: Number(version), active: active === 'true', readAt };
}

/** The stage one rental stands in, which a read of the collection must leave alone. */
export function storedStage(rentalId) {
  return sql(`SELECT stage FROM rentals WHERE id = '${rentalId}'`);
}

/** The cursor the first page of a collection carries, which a check continues from. */
export async function firstCursorOf(account) {
  const answer = await notificationsOf(account, '?limit=1');
  assert.equal(answer.status, 200, answer.text);
  assert.match(answer.json.next_cursor, CURSOR_ALPHABET, 'the collection offered no cursor');
  return answer.json.next_cursor;
}

/** The identifier of the account behind one address, which no private answer publishes. */
export function ownerOf(account) {
  return sql(`SELECT id FROM users WHERE email = '${account.email}'`);
}

/** Registers a fresh account, which is the only way these suites obtain a signed-in person. */
export function newSuiteAccount(name) {
  return newAccount(`${ACCOUNT_PREFIX}-${name}`);
}

/**
 * Removes everything this suite wrote. A notification is named by the signal that announces it and
 * belongs to the rental it describes, so the signals go first and the rentals after them; the
 * reservations the suite made are then given back and deleted the way the reservation suites do it.
 */
export async function endSuiteNotifications() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(
    `DELETE FROM outbox
     WHERE recipient_id IN ${mine}
        OR resource_id IN (SELECT id FROM notifications WHERE user_id IN ${mine})`,
  );
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
  await endSuiteReservations();
}

export { sql };
export { issueCursor, positionOf, tamper } from './cursors.mjs';

/** One vehicle of the prepared demonstration, which a written rental only has to name. */
function anyVehicle() {
  return sql('SELECT id FROM vehicles ORDER BY id LIMIT 1');
}
