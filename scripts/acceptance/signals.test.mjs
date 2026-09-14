// The personal change signal of a notification, observed on the assembled stack: the frame the
// owner's own subscription receives, how long it took to arrive measured from the moment the
// database stored the warning, and the streams that never see it.
//
// The signal is delivered by the worker. The deadline pass creates the warning and records the task
// that announces it in the same transaction; the delivery publishes it in the PostgreSQL channel;
// the API writes it to the subscriptions it is addressed to. Nothing here is read by polling, and
// the same delivery is checked in a browser in `tests/e2e/notifications.spec.mjs`, where the
// periodic reconciliation is switched off for the same reason.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { after, before, beforeEach, describe, test } from 'node:test';
import { setTimeout as delay } from 'node:timers/promises';
import { sql, waitForReady } from './client.mjs';
import { changeOf, closeStream, frameText, waitForFrame, watchEventStream } from './events.mjs';
import { endSuiteNotifications, ownerOf } from './notifications.mjs';
import { availableVehicle, newAccount, until } from './reservations.mjs';
import { insertRental } from './rentalrows.mjs';

/** Every account this suite registers carries this prefix, which is also how its rows are found. */
const ACCOUNT_PREFIX = 'signals';

/** The two streams of a client: the public one no account owns, and the one a session carries. */
const PUBLIC_EVENTS_PATH = '/api/v1/events';
const PRIVATE_EVENTS_PATH = '/api/v1/me/events';

/** The change the worker announces when a notification of an account is created or changed. */
const NOTIFICATION_CHANGED = 'notification.changed';

/**
 * The bound a committed change must reach its owner within, measured from the moment the database
 * stored the warning. It is the two seconds the acceptance criteria state for the local environment.
 */
const DELIVERY_BOUND_MS = 2000;

/** How long a check waits for the deadline pass to warn the reservation it prepared. */
const WARNING_PATIENCE_MS = 20_000;

/** How long a check waits before it believes that a stream carried nothing. */
const SILENCE_MS = 1500;

/**
 * How long before its deadline the prepared reservation is placed, which is inside the warning window
 * of the rule and therefore due for a warning on the next pass.
 */
const INSIDE_LAST_MINUTE_SECONDS = 30;

/** How long the reservation the check prepares has stood, stated from the clock of the database. */
const RESERVATION_STARTED_SECONDS_AGO = 840;

before(async () => {
  await waitForReady();
});

// A check that left a reservation behind would hold a vehicle the next one needs, and the suites
// after this one read the prepared demonstration. This suite's accounts carry the notification
// prefix, so the cleanup that already knows those rows is the one it uses.
beforeEach(endSuiteNotifications);

after(endSuiteNotifications);

describe('the personal change signal', () => {
  test('reaches its owner inside two seconds of the moment the warning was stored', async () => {
    const owner = await newAccount(`${ACCOUNT_PREFIX}-owner`);
    const stream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: owner.cookie });
    await waitForFrame(stream, (frame) => frame.event === 'ready');

    try {
      const rentalId = prepareReservation(owner, await availableVehicle());
      const warningId = await until(
        () => notificationOf(rentalId),
        'the deadline pass never warned the reservation',
        WARNING_PATIENCE_MS,
      );

      // The warning exists before its frame is waited for, so the wait below is for a delivery that
      // has already been recorded rather than for the creation of the warning itself. The measurement
      // is reported as well as asserted, so a run says what the delivery actually cost.
      const frame = await arrived(stream, warningId);
      const deliveredIn = Date.now() - storedWarningMoment(warningId);
      console.log(`the signal reached its owner ${deliveredIn} ms after the warning was stored`);
      assert.ok(
        deliveredIn < DELIVERY_BOUND_MS,
        `the signal reached its owner ${deliveredIn} ms after the warning was stored`,
      );

      // The frame states the version the stored notification reached, which is the version a client
      // reads the collection for, and the task that carried it names the owner as its only recipient.
      assert.equal(changeOf(frame).version, '1', 'the frame announced another version');
      assert.equal(storedVersion(warningId), 1, 'the warning was not created at its first version');
      assert.equal(
        sql(`SELECT recipient_id FROM outbox WHERE resource_id = '${warningId}'`),
        await ownerOf(owner),
        'the signal was addressed to somebody other than the owner',
      );
    } finally {
      closeStream(stream);
    }
  });

  test('never reaches another account and is never published', async () => {
    const owner = await newAccount(`${ACCOUNT_PREFIX}-private-owner`);
    const stranger = await newAccount(`${ACCOUNT_PREFIX}-stranger`);
    const ownerStream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: owner.cookie });
    const strangerStream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: stranger.cookie });
    const publicStream = await watchEventStream(PUBLIC_EVENTS_PATH);
    await waitForFrame(ownerStream, (frame) => frame.event === 'ready');
    await waitForFrame(strangerStream, (frame) => frame.event === 'ready');
    await waitForFrame(publicStream, (frame) => frame.event === 'ready');

    try {
      const rentalId = prepareReservation(owner, await availableVehicle());
      const warningId = await until(
        () => notificationOf(rentalId),
        'the deadline pass never warned the reservation',
        WARNING_PATIENCE_MS,
      );

      // The owner receiving that very change is what makes the two absences below meaningful rather
      // than a stream that simply carried nothing.
      await arrived(ownerStream, warningId);
      await delay(SILENCE_MS);

      assert.equal(
        strangerStream.frames.some(isNotificationChanged),
        false,
        'another account received a private notification signal',
      );
      assert.equal(
        publicStream.frames.some(isNotificationChanged),
        false,
        'the public stream carried a private notification signal',
      );
    } finally {
      closeStream(ownerStream);
      closeStream(strangerStream);
      closeStream(publicStream);
    }
  });
});

/** The frame one stream carried about one notification, or a failure naming the frames it did carry. */
async function arrived(stream, notificationId) {
  return until(
    () => stream.frames.find((frame) => frame.event === NOTIFICATION_CHANGED && changeOf(frame)?.id === notificationId),
    `the stream carried no signal about ${notificationId}; it carried ${stream.frames.map(frameText).join(' | ')}`,
  );
}

/** Whether one frame is a personal notification change, whoever it was about. */
function isNotificationChanged(frame) {
  return frame.event === NOTIFICATION_CHANGED;
}

/**
 * Writes one reservation inside its last minute, directly and relative to the clock of the database,
 * which is how a check reaches a deadline the commands of this build cannot produce on demand. The
 * deadline pass warns it exactly as it warns a reservation made through the service, because the
 * selection reads the stored moments rather than the way the row was written.
 */
function prepareReservation(account, vehicleId) {
  const id = randomUUID();
  sql(
    insertRental({
      id,
      email: account.email,
      vehicleId,
      stage: 'reserved',
      reservedAt: `clock_timestamp() - make_interval(secs => ${RESERVATION_STARTED_SECONDS_AGO})`,
      expiresAt: `clock_timestamp() + make_interval(secs => ${INSIDE_LAST_MINUTE_SECONDS})`,
    }),
  );
  return id;
}

/** The identifier of the warning the database holds for one rental, or '' while it holds none. */
function notificationOf(rentalId) {
  return sql(`SELECT id FROM notifications WHERE rental_id = '${rentalId}'`);
}

/** When the database stored one warning, which is the moment its delivery is measured from. */
function storedWarningMoment(notificationId) {
  const query = `SELECT (extract(epoch FROM created_at) * 1000)::bigint FROM notifications WHERE id = '${notificationId}'`;
  return Number(sql(query));
}

/** The version one stored notification reached. */
function storedVersion(notificationId) {
  return Number(sql(`SELECT version FROM notifications WHERE id = '${notificationId}'`));
}
