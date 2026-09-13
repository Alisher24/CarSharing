// The change signals of T07, observed against the assembled stack: the handshake and the frames of a
// stream, a committed change reaching a connected client, the privacy of the two streams, the session
// a private stream keeps proving, and the queue that survives a worker that is stopped, restarted and
// failing.
//
// Everything here goes through the published origin, so the frames a browser would receive are the
// frames these checks read.
import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';
import { setTimeout as delay } from 'node:timers/promises';
import {
  call,
  compose,
  password,
  registerAccount,
  resetRateLimits,
  sessionCookie,
  SIGN_OUT_PATH,
  sql,
  waitForReady,
} from './client.mjs';
import {
  changeOf,
  closeStream,
  frameText,
  watchEventStream,
  waitForEnd,
  waitForFrame,
  STREAM_PATIENCE_MS,
} from './events.mjs';
import { rentalConditions } from './rentalrows.mjs';

const PUBLIC_EVENTS_PATH = '/api/v1/events';
const PRIVATE_EVENTS_PATH = '/api/v1/me/events';
const VEHICLES_PATH = '/api/v1/vehicles';
const ME_PATH = '/api/v1/me';

/** The timestamps a ready frame states: UTC, six fractional digits, exactly as the contract declares. */
const TIMESTAMP_PATTERN = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;

/** The retry hint the handshake declares, in milliseconds. */
const RETRY_HINT_MS = 3000;

/**
 * The bound a change must reach a connected client within, measured from the moment the database
 * committed it. It is the two seconds the acceptance criteria state for the local environment.
 */
const DELIVERY_BOUND_MS = 2000;

/** How long an ordinary vehicle change waits for its signal: the sweep, the claim and the write. */
const ARRIVAL_PATIENCE_MS = 8000;

/** The query the run instructions document for looking at tasks that keep failing. */
const PROBLEM_JOBS_QUERY =
  'SELECT id, kind, attempts, last_error, next_attempt_at FROM outbox ' +
  'WHERE completed_at IS NULL AND attempts >= 10 ORDER BY next_attempt_at';

/** The accounts and the rentals this suite creates, so it can leave the demonstration as it found it. */
const createdUsers = [];
const createdTasks = [];

describe('change signals', () => {
  let owner;
  let stranger;

  before(async () => {
    // This suite signs in repeatedly to revoke a session, and every suite shares one address, so it
    // starts from the limits the last run left rather than from a budget already spent.
    resetRateLimits();
    owner = await registerAccount('events-owner');
    assert.equal(owner.response.status, 201, owner.response.text);
    createdUsers.push(await userIdOf(owner.cookie));

    stranger = await registerAccount('events-stranger');
    assert.equal(stranger.response.status, 201, stranger.response.text);
    createdUsers.push(await userIdOf(stranger.cookie));
  });

  after(async () => {
    // A person's rental on a scenario vehicle stops the demonstration restoration, so this suite
    // leaves none behind.
    if (createdUsers.length > 0) {
      sql(`DELETE FROM rentals WHERE user_id = ANY('{${createdUsers.join(',')}}'::uuid[])`);
    }
    if (createdTasks.length > 0) {
      sql(`DELETE FROM outbox WHERE id = ANY('{${createdTasks.join(',')}}'::uuid[])`);
    }
  });

  test('answers an anonymous stream with the contract handshake', async () => {
    const stream = await watchEventStream(PUBLIC_EVENTS_PATH);
    try {
      assert.equal(stream.status, 200, stream.body);
      assert.equal(stream.headers.get('content-type'), 'text/event-stream');
      assert.equal(stream.headers.get('cache-control'), 'no-store');
      assert.ok(stream.headers.get('x-request-id'), 'the stream states no request identifier');

      await waitForFrame(stream, (frame) => frame.event === 'ready');
      const [handshake, ...rest] = stream.frames;
      assert.equal(handshake.retry, RETRY_HINT_MS, 'the handshake states no retry hint');
      assert.equal(handshake.event, 'ready');
      assert.match(JSON.parse(handshake.data).server_time, TIMESTAMP_PATTERN);
      for (const frame of rest) {
        assert.equal(frame.id, null, 'the stream sent an event identifier, which it must not');
      }
    } finally {
      closeStream(stream);
    }
  });

  test('reaches a connected client within two seconds of the commit', async () => {
    const publicStream = await watchEventStream(PUBLIC_EVENTS_PATH);
    const privateStream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: owner.cookie });
    try {
      await waitForFrame(publicStream, (frame) => frame.event === 'ready');
      await waitForFrame(privateStream, (frame) => frame.event === 'ready');

      const reservation = await insertDueReservation(owner.cookie);

      const signal = await waitForFrame(
        publicStream,
        (frame) => changeOf(frame)?.id === reservation.vehicleId,
        ARRIVAL_PATIENCE_MS,
      );
      const deliveredMs = Date.now() - reservation.committedAt;
      assert.equal(signal.event, 'vehicle.changed');
      assert.ok(deliveredMs < DELIVERY_BOUND_MS, `the change reached the client ${deliveredMs} ms after the commit`);

      const own = await waitForFrame(
        privateStream,
        (frame) => changeOf(frame)?.id === reservation.id,
        ARRIVAL_PATIENCE_MS,
      );
      assert.equal(own.event, 'rental.changed');
    } finally {
      closeStream(publicStream);
      closeStream(privateStream);
    }
  });

  test('refuses the private stream to a caller who is not signed in', async () => {
    const refused = await watchEventStream(PRIVATE_EVENTS_PATH);
    assert.equal(refused.status, 401, refused.body);
    assert.equal(JSON.parse(refused.body).code, 'AUTHENTICATION_REQUIRED');
    assert.equal(refused.frames.length, 0);
  });

  test('does not carry one account’s change to another, and publishes no rental publicly', async () => {
    const strangerStream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: stranger.cookie });
    const publicStream = await watchEventStream(PUBLIC_EVENTS_PATH);
    const ownerStream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: owner.cookie });
    try {
      // Every stream is connected before the change is made: a signal is not replayed, so a stream
      // that opened later would observe nothing and prove nothing about who may hear what.
      await waitForFrame(strangerStream, (frame) => frame.event === 'ready');
      await waitForFrame(publicStream, (frame) => frame.event === 'ready');
      await waitForFrame(ownerStream, (frame) => frame.event === 'ready');

      const reservation = await insertDueReservation(owner.cookie);

      // The owner's change is delivered, which is what makes the absence below meaningful rather than
      // a stream that simply carried nothing.
      await waitForFrame(ownerStream, (frame) => changeOf(frame)?.id === reservation.id, ARRIVAL_PATIENCE_MS);
      await delay(1000);
      assert.equal(
        strangerStream.frames.some((frame) => changeOf(frame)?.id === reservation.id),
        false,
        'another account received a private change',
      );
      assert.equal(
        publicStream.frames.some((frame) => frame.event === 'rental.changed'),
        false,
        'the public stream published a private change',
      );
      // The private stream is not a second copy of the public one: its holder reads the public
      // snapshot itself, and the two streams do not overlap.
      assert.equal(
        ownerStream.frames.some((frame) => frame.event === 'vehicle.changed'),
        false,
        'the private stream duplicated a public change',
      );
    } finally {
      closeStream(strangerStream);
      closeStream(publicStream);
      closeStream(ownerStream);
    }
  });

  test('closes the private stream when the session is revoked', async () => {
    const stream = await watchEventStream(PRIVATE_EVENTS_PATH, { cookie: owner.cookie });
    try {
      assert.equal(stream.status, 200, stream.body);
      await waitForFrame(stream, (frame) => frame.event === 'ready');
      await revokeSession(owner);
      await waitForEnd(stream);
      assert.equal(stream.ended, true);
    } finally {
      closeStream(stream);
    }
  });

  test('keeps a change until the worker runs, and delivers it when one does', async () => {
    compose('stop', 'worker');
    const stream = await watchEventStream(PUBLIC_EVENTS_PATH);
    try {
      await waitForFrame(stream, (frame) => frame.event === 'ready');
      const reservation = await insertDueReservation(owner.cookie);

      await delay(3000);
      assert.equal(
        stream.frames.some((frame) => changeOf(frame)?.id === reservation.vehicleId),
        false,
        'a change was delivered while no worker was running',
      );

      compose('start', 'worker');
      await waitForFrame(stream, (frame) => changeOf(frame)?.id === reservation.vehicleId, ARRIVAL_PATIENCE_MS);
    } finally {
      closeStream(stream);
      compose('start', 'worker');
    }
  });

  test('carries every public kind the contract declares', async () => {
    const stream = await watchEventStream(PUBLIC_EVENTS_PATH);
    try {
      await waitForFrame(stream, (frame) => frame.event === 'ready');
      for (const kind of ['zone.changed', 'tariff.changed']) {
        const task = insertSignalTask(kind);
        const frame = await waitForFrame(
          stream,
          (carried) => changeOf(carried)?.id === task.resourceId,
          ARRIVAL_PATIENCE_MS,
        );
        assert.equal(frame.event, kind, frameText(frame));
        assert.equal(changeOf(frame).version, String(SIGNAL_VERSION));
      }
    } finally {
      closeStream(stream);
    }
  });

  test('closes a served stream when the API is restarted, and serves a new one', async () => {
    const stream = await watchEventStream(PUBLIC_EVENTS_PATH);
    await waitForFrame(stream, (frame) => frame.event === 'ready');

    compose('restart', 'api');
    try {
      await waitForEnd(stream);
    } finally {
      closeStream(stream);
      await waitForReady();
    }

    // A new connection announces itself again rather than replaying what it missed: the frames it
    // receives start with a handshake, and the versions it is told are newer than the ones the
    // previous connection had already seen.
    const reconnected = await watchEventStream(PUBLIC_EVENTS_PATH);
    try {
      const handshake = await waitForFrame(reconnected, (frame) => frame.event === 'ready');
      assert.equal(reconnected.frames[0], handshake, 'the first frame of a new stream was not its handshake');
      const lastSeen = Math.max(...stream.frames.map((frame) => Number(changeOf(frame)?.version ?? 0)));
      const next = await waitForFrame(
        reconnected,
        (frame) => Number(changeOf(frame)?.version ?? 0) > lastSeen,
        ARRIVAL_PATIENCE_MS,
      );
      assert.ok(Number(changeOf(next).version) > lastSeen);
    } finally {
      closeStream(reconnected);
    }
  });

  test('delivers each task once while two workers claim from the same queue', async () => {
    compose('up', '--detach', '--scale', 'worker=2', 'worker');
    try {
      // The tasks are written in one statement and are due immediately, so both workers have the
      // whole queue to compete for.
      const written = insertTasks(24);
      for (const id of written) {
        await waitForTask(id, (task) => task.completed_at !== '', 30_000);
        assert.equal((await taskRow(id)).attempts, '1', `task ${id} was claimed more than once`);
      }
    } finally {
      compose('up', '--detach', '--scale', 'worker=1', 'worker');
    }
  });

  test('keeps a task of an unknown kind with its error, and delivers it once it is understood', async () => {
    const id = insertTask({ kind: 'demonstration.unknown' });
    await waitForTask(id, (task) => Number(task.attempts) >= 1);
    const failed = await taskRow(id);
    assert.equal(failed.completed_at, '', 'an unknown kind was reported as delivered');
    assert.match(failed.last_error, /no delivery is declared/);

    // The task was preserved, so the delivery it names can still be performed once it is understood.
    sql(`UPDATE outbox SET kind = 'vehicle.changed', next_attempt_at = now() WHERE id = '${id}'`);
    await waitForTask(id, (task) => task.completed_at !== '');
  });

  test('reports a task that has failed ten times, and keeps it', async () => {
    const id = insertTask({ kind: 'demonstration.unknown', attempts: 9 });
    await waitForTask(id, (task) => Number(task.attempts) >= 10);

    const problemJobs = sql(PROBLEM_JOBS_QUERY);
    assert.match(problemJobs, new RegExp(id), 'the documented diagnostics query does not list the task');
    const failed = await taskRow(id);
    assert.equal(failed.completed_at, '');
    assert.match(failed.last_error, /no delivery is declared/);
  });

  test('holds a claimed task for its lease and takes it over once the lease has run out', async () => {
    const id = insertTask({
      kind: 'vehicle.changed',
      leaseToken: '11111111-1111-4111-8111-111111111111',
      leaseSeconds: 30,
    });
    await delay(2000);
    const held = await taskRow(id);
    assert.equal(held.attempts, '0', 'a task under a live lease was claimed by another attempt');
    assert.equal(held.completed_at, '');

    // The lease rule every settlement carries: an attempt that does not hold the task's token changes
    // nothing about it.
    const stale =
      `UPDATE outbox SET last_error = 'stale attempt' WHERE id = '${id}'` +
      " AND lease_token = '22222222-2222-4222-8222-222222222222' AND lease_expires_at > now()";
    sql(stale);
    assert.equal((await taskRow(id)).last_error, '', 'a stale attempt rewrote the task');

    sql(`UPDATE outbox SET lease_expires_at = now() - interval '1 second' WHERE id = '${id}'`);
    await waitForTask(id, (task) => task.completed_at !== '');
    assert.equal((await taskRow(id)).attempts, '1', 'the task was not claimed exactly once more');
  });

  test('deletes delivered signal tasks once their retention has passed, and nothing else', async () => {
    const expired = insertTask({ kind: 'vehicle.changed', completedHoursAgo: 25 });
    const recent = insertTask({ kind: 'vehicle.changed', completedHoursAgo: 1 });
    const unfinished = insertTask({ kind: 'demonstration.unknown' });
    const otherPurpose = insertTask({ kind: 'invoice.email', completedHoursAgo: 25 });

    await waitForTask(expired, (task) => task.deleted, 30_000);

    assert.ok(await taskRow(recent), 'a task inside its retention was deleted');
    assert.ok(await taskRow(otherPurpose), 'a task of another purpose was deleted');
    const kept = await taskRow(unfinished);
    assert.equal(kept.completed_at, '', 'an undelivered task was deleted');
    assert.match(kept.last_error, /no delivery is declared/);
  });

  test('keeps a quiet connection alive with a comment', async () => {
    const stream = await watchEventStream(PUBLIC_EVENTS_PATH);
    try {
      await waitForFrame(stream, (frame) => frame.event === 'ready');
      const keepalive = await waitForFrame(stream, (frame) => frame.comment === 'keepalive', STREAM_PATIENCE_MS);
      assert.equal(keepalive.event, null, 'the heartbeat was sent as an event');
    } finally {
      closeStream(stream);
    }
  });

  test('keeps answering REST while a stream is closed', async () => {
    const stream = await watchEventStream(PUBLIC_EVENTS_PATH);
    await waitForFrame(stream, (frame) => frame.event === 'ready');
    closeStream(stream);
    await waitForEnd(stream);

    const catalog = await call(VEHICLES_PATH);
    assert.equal(catalog.status, 200, catalog.text);
    assert.ok(Array.isArray(catalog.json.items));
  });
});

/** The account identifier behind a session, which the suite needs to attribute a rental to it. */
async function userIdOf(cookie) {
  const response = await call(ME_PATH, { cookie });
  assert.equal(response.status, 200, response.text);
  return response.json.user.id;
}

/**
 * Records a reservation that has already run out and returns what the checks need: the row it wrote,
 * the vehicle it holds and the instant the database committed it, which is what a delivery is
 * measured from.
 *
 * The reservation is written directly because booking over HTTP belongs to a later task; the release
 * it triggers is the module's own transition, performed by the worker.
 */
async function insertDueReservation(cookie) {
  const userId = await userIdOf(cookie);
  const written = sql(
    `WITH free_vehicle AS (
       SELECT vehicle.id FROM vehicles vehicle
       LEFT JOIN rentals live ON live.vehicle_id = vehicle.id AND live.ended_at IS NULL
       WHERE live.id IS NULL
       ORDER BY vehicle.id
       LIMIT 1
     ), price AS (
       SELECT * FROM tariffs ORDER BY id LIMIT 1
     ), written AS (
       INSERT INTO rentals (
         id, user_id, vehicle_id, stage, tariff_id, zone_id, reserved_at, expires_at, version,${rentalConditions.columns}
       )
       SELECT gen_random_uuid(), '${userId}', free_vehicle.id, 'reserved',
              price.id,
              (SELECT id FROM service_zones ORDER BY id LIMIT 1),
              now() - interval '20 minutes', now() - interval '5 minutes', 1,${rentalConditions.values}
       FROM free_vehicle, price
       RETURNING id, vehicle_id
     )
     SELECT id, vehicle_id, (extract(epoch FROM clock_timestamp()) * 1000)::bigint FROM written`,
  );
  const [id, vehicleId, committedAt] = written.split('|');
  assert.ok(id && vehicleId, `no free vehicle took the reservation: ${written}`);
  return { id, vehicleId, committedAt: Number(committedAt) };
}

/** Signs the account out, which is what a browser does when its holder leaves. */
async function revokeSession(account) {
  const current = await call(ME_PATH, { cookie: account.cookie });
  assert.equal(current.status, 200, current.text);
  const response = await call(SIGN_OUT_PATH, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: current.json.csrf_token,
  });
  assert.equal(response.status, 204, response.text);

  // The suite keeps using the account afterwards, so it signs in again with the session it replaced.
  const again = await call('/api/v1/auth/login', {
    method: 'POST',
    body: { email: account.email, password },
  });
  assert.equal(again.status, 200, again.text);
  account.cookie = sessionCookie(again);
}

/** Writes one task into the queue, so a check can observe what the worker does with it. */
function insertTask({ kind, attempts = 0, leaseToken = null, leaseSeconds = 0, completedHoursAgo = null }) {
  const lease = leaseToken ? `'${leaseToken}', now() + interval '${leaseSeconds} seconds'` : 'NULL, NULL';
  const completed = completedHoursAgo === null ? 'NULL' : `now() - interval '${completedHoursAgo} hours'`;
  const id = sql(
    `WITH written AS (
       INSERT INTO outbox (
         kind, resource_id, version, recipient_id, attempts, lease_token, lease_expires_at, completed_at
       )
       VALUES ('${kind}', gen_random_uuid(), 1, NULL, ${attempts}, ${lease}, ${completed})
       RETURNING id
     )
     SELECT id FROM written`,
  );
  createdTasks.push(id);
  return id;
}

/** The version a task written by a check announces, so a frame can be recognized by it. */
const SIGNAL_VERSION = 7;

/** Writes one deliverable signal task and names the resource whose change a stream must carry. */
function insertSignalTask(kind) {
  const written = sql(
    `WITH written AS (
       INSERT INTO outbox (kind, resource_id, version, recipient_id)
       VALUES ('${kind}', gen_random_uuid(), ${SIGNAL_VERSION}, NULL)
       RETURNING id, resource_id
     )
     SELECT id, resource_id FROM written`,
  );
  const [id, resourceId] = written.split('|');
  createdTasks.push(id);
  return { id, resourceId };
}

/** Writes several deliverable signal tasks at once, so two workers have a queue to compete for. */
function insertTasks(count) {
  const written = sql(
    `WITH written AS (
       INSERT INTO outbox (kind, resource_id, version, recipient_id)
       SELECT 'vehicle.changed', gen_random_uuid(), 1, NULL FROM generate_series(1, ${count})
       RETURNING id
     )
     SELECT string_agg(id::text, ',') FROM written`,
  );
  const ids = written === '' ? [] : written.split(',');
  createdTasks.push(...ids);
  return ids;
}

/** One task as the checks read it, or null once it is no longer in the queue. */
async function taskRow(id) {
  const row = sql(
    `SELECT kind, attempts, coalesce(last_error, ''), coalesce(completed_at::text, '')
     FROM outbox WHERE id = '${id}'`,
  );
  if (row === '') return null;
  const [kind, attempts, last_error, completed_at] = row.split('|');
  return { kind, attempts, last_error, completed_at };
}

/** Waits until a task reaches the state a check asserts about, so a slow worker is not a failure. */
async function waitForTask(id, reached, patienceMs = STREAM_PATIENCE_MS) {
  const deadline = Date.now() + patienceMs;
  for (;;) {
    const task = await taskRow(id);
    if (task !== null && reached({ ...task, deleted: false })) return task;
    if (task === null && reached({ deleted: true })) return null;
    if (Date.now() > deadline) {
      throw new Error(`the task did not reach the expected state within ${patienceMs} ms: ${JSON.stringify(task)}`);
    }
    await delay(100);
  }
}
