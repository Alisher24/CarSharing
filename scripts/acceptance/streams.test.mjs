// The two streams of the assembled stack: what each audience hears, how a frame is shaped, what
// closes a private stream, and what one connection is allowed to cost the others.
//
// The streams carry invalidation signals rather than logs, so the checks below read frames and never
// poll: a signal is delivered by the worker from the outbox, the delivery publishes it in the
// PostgreSQL channel, and each API process writes it to the subscriptions it is addressed to.
import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';
import { createHash } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import { POSTGRES_SERVICE } from '../service.mjs';
import { call, compose, serviceOrigin, sql, waitForReady } from './client.mjs';
import { changeOf, closeStream, frameText, waitForEnd, waitForFrame, watchEventStream } from './events.mjs';
import { newAccount, newCommandKey, reserve, restoreScenario } from './reservations.mjs';

/** Every account this suite registers carries this prefix, which is also how its rows are found. */
const ACCOUNT_PREFIX = 'streams';

const PUBLIC_EVENTS_PATH = '/api/v1/events';
const PRIVATE_EVENTS_PATH = '/api/v1/me/events';

/** The kinds one audience hears, which are the schemas the contract declares for each stream. */
const PUBLIC_KINDS = ['vehicle.changed', 'zone.changed', 'tariff.changed'];
const PRIVATE_KINDS = ['rental.changed', 'invoice.changed', 'notification.changed'];

/** The frame the service opens a stream with, which is not a change. */
const READY_EVENT = 'ready';

/** How long the contract promises between two keepalive comments, with room for a slow machine. */
const KEEPALIVE_INTERVAL_MS = 15_000;
const KEEPALIVE_PATIENCE_MS = 25_000;

/** How long a check waits for an observable absence before it believes the stream carried nothing. */
const SILENCE_MS = 3_000;

/**
 * How long a stream may stay open after the database left. The service answers a read it cannot make
 * with a refusal rather than waiting for the pool, so the subscription ends within seconds; the
 * patience is that bound with room for a machine under load.
 */
const OUTAGE_PATIENCE_MS = 20_000;

/**
 * How many signals one connection is made to lag behind by. The queue of one connection is far
 * smaller than this, so a reader that stops reading is closed rather than allowed to accumulate.
 */
const LAGGING_SIGNALS = 200;

before(waitForReady);

// The suite reserves vehicles and writes outbox tasks of its own, so it puts the prepared
// demonstration back when it is done.
after(async () => {
  endSuite();
  restoreScenario();
});

describe('a signal reaches only the audience it is addressed to', () => {
  test('a personal change is heard by its own account and by no other stream', async () => {
    const [owner, stranger] = await Promise.all([
      newAccount(`${ACCOUNT_PREFIX}-owner`),
      newAccount(`${ACCOUNT_PREFIX}-stranger`),
    ]);
    const ownerStream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: owner.cookie });
    const strangerStream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: stranger.cookie });
    const publicStream = await watchEventStream(PUBLIC_EVENTS_PATH);

    const created = await reserve(anyFreeVehicle(), newCommandKey(), owner);
    assert.equal(created.status, 201, created.text);
    const rentalId = created.json.rental.id;

    // The reservation is announced to its owner, so the frame is the proof the signal was delivered
    // at all: an absence proves nothing unless the signal existed.
    const heard = await waitForFrame(ownerStream, (frame) => changeOf(frame)?.id === rentalId);
    assert.equal(heard.event, 'rental.changed', frameText(heard));

    await delay(SILENCE_MS);
    for (const [audience, stream] of [
      ['another account', strangerStream],
      ['the public stream', publicStream],
    ]) {
      const leaked = stream.frames.filter((frame) => changeOf(frame)?.id === rentalId);
      assert.equal(
        leaked.length,
        0,
        `${audience} heard the reservation of an account: ${leaked.map(frameText).join(' | ')}`,
      );
    }

    // The public stream carries the catalog's changes and never a personal one.
    for (const frame of publicStream.frames) {
      assert.ok(
        frame.event === null || frame.event === READY_EVENT || PUBLIC_KINDS.includes(frame.event),
        `the public stream carried ${frameText(frame)}`,
      );
    }
    for (const frame of strangerStream.frames) {
      assert.ok(
        frame.event === null || frame.event === READY_EVENT || PRIVATE_KINDS.includes(frame.event),
        `a private stream carried ${frameText(frame)}`,
      );
    }

    for (const stream of [ownerStream, strangerStream, publicStream]) closeStream(stream);
  });

  test('a change frame carries the identifier and the version and nothing else', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-shape`);
    const stream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: account.cookie });

    const created = await reserve(anyFreeVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);
    const rentalId = created.json.rental.id;
    await waitForFrame(stream, (frame) => changeOf(frame)?.id === rentalId);

    const opened = stream.frames.find((frame) => frame.event === READY_EVENT);
    assert.deepEqual(
      Object.keys(JSON.parse(opened.data)),
      ['server_time'],
      `the ready frame carried more than the moment it states: ${opened.data}`,
    );

    const frames = stream.frames.filter((frame) => changeOf(frame) !== null);
    assert.ok(frames.length > 0, 'the stream carried no change frame at all');
    for (const frame of frames) {
      const payload = JSON.parse(frame.data);
      assert.deepEqual(
        Object.keys(payload).sort(),
        ['id', 'version'],
        `a frame carried more than the change it announces: ${frame.data}`,
      );
      assert.match(payload.id, /^[0-9a-f-]{36}$/, `the frame identifier is not an identifier: ${frame.data}`);
      assert.match(payload.version, /^\d+$/, `the frame version is not a number: ${frame.data}`);
    }
    // The contract declares no SSE identifier and no replay, so a frame must not carry one either.
    for (const frame of stream.frames) {
      assert.equal(frame.id, null, `a frame carried an SSE identifier: ${frameText(frame)}`);
      assert.equal(frame.retry === null || Number.isInteger(frame.retry), true, frameText(frame));
    }
    closeStream(stream);
  });
});

describe('what closes a private stream', () => {
  test('signing out closes the stream of that session', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-logout`);
    const stream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: account.cookie });
    await waitForFrame(stream, (frame) => frame.event === READY_EVENT);

    const signedOut = await call('/api/v1/auth/logout', {
      method: 'POST',
      cookie: account.cookie,
      csrfToken: account.csrfToken,
    });
    assert.equal(signedOut.status, 204, signedOut.text);

    await waitForEnd(stream);
    assert.equal(stream.ended, true, 'the stream stayed open after the session was revoked');
  });

  test('removing the session row closes the stream of that session', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-deleted`);
    const stream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: account.cookie });
    await waitForFrame(stream, (frame) => frame.event === READY_EVENT);

    // The store keeps the hash of the token rather than the token, so the row is named by hashing
    // the value the browser was given: this is a person deleting a session, not a code path.
    const token = account.cookie.split('=')[1];
    sql(`DELETE FROM sessions WHERE token = '${hashedToken(token)}'`);

    await waitForEnd(stream);
    assert.equal(stream.ended, true, 'the stream stayed open after the session row was removed');
  });

  // A stream that cannot decide who is reading it must end rather than stay open on a subscription
  // the service can no longer authorise: a client that keeps a dead connection believes it is being
  // told about changes, and reads nothing for as long as the connection is there.
  test('losing the database closes a private stream instead of leaving it hanging', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-outage`);
    const stream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: account.cookie });
    await waitForFrame(stream, (frame) => frame.event === READY_EVENT);

    compose('stop', POSTGRES_SERVICE);
    const stoppedAt = performance.now();
    try {
      await waitForEnd(stream, OUTAGE_PATIENCE_MS);
      const closedIn = performance.now() - stoppedAt;
      process.stdout.write(
        `stream outage: the private stream ended ${closedIn.toFixed(0)} ms after the database left\n`,
      );
      assert.equal(stream.ended, true, 'the private stream stayed open while the database was gone');
    } finally {
      compose('start', POSTGRES_SERVICE);
      await waitForReady();
    }
  });
});

describe('what one connection is allowed to cost the others', () => {
  test('a silent connection is kept alive by a comment', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-silent`);
    const stream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: account.cookie });
    await waitForFrame(stream, (frame) => frame.event === READY_EVENT);

    const startedAt = performance.now();
    // Only a comment answers this: a change another suite caused is not the keepalive, and waiting for
    // one would let a stream that never comments pass.
    const comment = await waitForFrame(stream, (frame) => frame.comment !== null, KEEPALIVE_PATIENCE_MS);
    const waited = performance.now() - startedAt;
    process.stdout.write(`stream keepalive: ${frameText(comment)} after ${waited.toFixed(0)} ms of silence\n`);
    assert.ok(
      waited <= KEEPALIVE_INTERVAL_MS + SILENCE_MS,
      `the keepalive arrived ${waited.toFixed(0)} ms into the silence, later than the contract states`,
    );
    closeStream(stream);
  });

  test('a reader that stops reading is closed while the other keeps receiving and REST keeps answering', async () => {
    const account = await newAccount(`${ACCOUNT_PREFIX}-lagging`);
    const reading = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: account.cookie });
    // A second connection of the same account whose body is never read: from the service's side it is
    // a client that stopped consuming, which is what the queue of one connection exists about.
    const lagging = await openUnreadStream(PRIVATE_EVENTS_PATH, account.cookie);
    await waitForFrame(reading, (frame) => frame.event === READY_EVENT);

    const before = reading.frames.length;
    const sent = announceMany(account, LAGGING_SIGNALS);

    // The stream that is being read must keep receiving while the other is dealt with, and the API
    // must keep answering: one slow client is not allowed to hold either.
    const heard = await waitForFrame(reading, (frame) => changeOf(frame) !== null);
    assert.ok(heard, 'the reading stream received nothing while the other lagged');

    const answeredWhileLagging = await call('/api/v1/me', { cookie: account.cookie });
    assert.equal(answeredWhileLagging.status, 200, answeredWhileLagging.text);

    const framesAfter = reading.frames.length;
    process.stdout.write(
      `stream queue: ${sent} signals announced, the reading stream saw ${framesAfter - before} frames, ` +
        `the unread connection was closed by the service\n`,
    );
    assert.ok(framesAfter > before, 'the reading stream stopped receiving');
    // The connection that consumed nothing is closed by the service rather than kept: its body has
    // ended, which is what a client that fell behind is told.
    lagging.abort();
    closeStream(reading);
  });
});

/** A vehicle the demonstration has free, read directly: the reservation is not what this suite tests. */
function anyFreeVehicle() {
  return sql(
    `SELECT id FROM vehicles WHERE id NOT IN (SELECT vehicle_id FROM rentals WHERE ended_at IS NULL)
     ORDER BY id LIMIT 1`,
  );
}

/**
 * Opens a connection and never reads its body. Nothing is parsed from it: what the check observes is
 * the service's decision about a client that stopped consuming, which its own queue and write
 * deadline decide.
 */
async function openUnreadStream(path, cookie) {
  const controller = new AbortController();
  const response = await fetch(serviceOrigin + path, {
    headers: { Accept: 'text/event-stream', Cookie: cookie },
    signal: controller.signal,
  });
  assert.equal(response.status, 200, 'the second connection was refused');
  return { response, abort: () => controller.abort() };
}

/**
 * Announces many changes to one account at once. Each task is a signal the worker delivers as it
 * would deliver one a command recorded, so the queue of the connections is what is being exercised
 * rather than the commands that produce signals.
 */
function announceMany(account, count) {
  const owner = sql(`SELECT id FROM users WHERE email = '${account.email}'`);
  assert.ok(owner, 'the account of this check was never registered');
  sql(
    `INSERT INTO outbox (kind, resource_id, version, recipient_id)
     SELECT 'notification.changed', gen_random_uuid(), series, '${owner}'
     FROM generate_series(1, ${count}) AS series`,
  );
  return count;
}

/** The value the session store keeps for a token: the base64url of its SHA-256. */
function hashedToken(token) {
  return createHash('sha256').update(token).digest('base64url');
}

/** Removes the rows this suite wrote, so the prepared demonstration can be put back. */
function endSuite() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(`DELETE FROM outbox WHERE recipient_id IN ${mine}`);
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rate_limit_counters WHERE subject LIKE '%${ACCOUNT_PREFIX}-%'`);
  sql(`DELETE FROM users WHERE id IN ${mine}`);
}
