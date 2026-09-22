// What the worker's sweeps do to the tables nothing else removes from: a sign-in counter, a session,
// and a command result. Each check writes a row that is inside its retention and a row that is past
// it, then waits for the sweep rather than calling it, because the sweep is what the running service
// does on its own.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import { before, describe, test } from 'node:test';
import {
  newEmail,
  registerAccount,
  resetRateLimits,
  signInRequest,
  SIGN_IN_PATH,
  call,
  sql,
  waitForReady,
} from './client.mjs';

const RATE_LIMIT_TABLE = 'rate_limit_counters';
const SESSION_TABLE = 'sessions';
const SIGN_IN_EMAIL_SCOPE = 'sign_in_email';

/** How long a check waits for a sweep, and how often it asks again. The sweeps run every ten seconds. */
const SWEEP_PATIENCE_MS = 40_000;
const SWEEP_POLL_MS = 500;

/** The retention the service documents for a counter and for a command result, in hours. */
const COUNTER_RETENTION_HOURS = 1;
const SESSION_LIFETIME_HOURS = 12;

before(() => waitForReady());

describe('the tables nothing else removes from are swept', () => {
  test('a counter inside its retention is kept, and the one past it is removed', async () => {
    resetRateLimits();
    const kept = `${newEmail('sweep-kept')}`;
    const removed = `${newEmail('sweep-removed')}`;
    writeCounter(SIGN_IN_EMAIL_SCOPE, kept, 0);
    writeCounter(SIGN_IN_EMAIL_SCOPE, removed, COUNTER_RETENTION_HOURS + 1);

    await untilSwept(() => counterExists(SIGN_IN_EMAIL_SCOPE, removed) === false, 'the expired counter was swept');
    assert.equal(
      counterExists(SIGN_IN_EMAIL_SCOPE, kept),
      true,
      'the sweep removed a counter still inside its retention',
    );

    // A counter the sweep left behind still refuses an attempt, which is the rule the sweep must not
    // break: the table is a budget, not a log.
    sql(`UPDATE ${RATE_LIMIT_TABLE} SET attempts = 30 WHERE scope = '${SIGN_IN_EMAIL_SCOPE}' AND subject = '${kept}'`);
    const refused = await call(SIGN_IN_PATH, signInRequest(kept));
    assert.equal(refused.status, 429, `a counter within its retention stopped refusing: ${refused.text}`);
  });

  test('a live session is kept, and an expired one is removed', async () => {
    resetRateLimits();
    const { cookie } = await registerAccount('sweep-session');
    const token = cookie.split('=')[1];
    const live = hashedToken(token);
    const expired = `expired-${Date.now()}`;

    // The row is written the way the store writes it: the hash of the token, the payload, and an
    // expiry. It starts live so the assertion that it landed cannot race the sweep, then is moved
    // into the past to become what a browser that never came back leaves behind.
    sql(
      `INSERT INTO ${SESSION_TABLE} (token, data, expiry)
       VALUES ('${expired}', '{}'::bytea, now() + interval '1 hour')`,
    );
    assert.equal(sessionExists(expired), true, 'the session was not written');
    sql(`UPDATE ${SESSION_TABLE} SET expiry = now() - interval '1 minute' WHERE token = '${expired}'`);

    await untilSwept(() => sessionExists(expired) === false, 'the expired session was swept');
    assert.equal(sessionExists(live), true, 'the sweep removed a live session');

    // The live session still works, which is the rule the sweep must not break.
    const me = await call('/api/v1/me', { cookie });
    assert.equal(me.status, 200, `a live session stopped working after the sweep: ${me.text}`);
  });

  test('the session sweep leaves a session that has not expired yet alone even when it is old', async () => {
    const aged = `old-${Date.now()}`;
    sql(
      `INSERT INTO ${SESSION_TABLE} (token, data, expiry)
       VALUES ('${aged}', '{}'::bytea, now() + interval '${SESSION_LIFETIME_HOURS} hours')`,
    );

    // One sweep is waited for, and the row that is only old is still there afterwards.
    await delay(SWEEP_POLL_MS * 4);
    assert.equal(sessionExists(aged), true, 'the sweep removed a session that had not expired');
    sql(`DELETE FROM ${SESSION_TABLE} WHERE token = '${aged}'`);
  });

  // The two checks above prove that a row inside its retention survives and a row past it goes. What
  // this one measures is the growth itself: how many rows of each table one burst of subjects adds and
  // what the sweep leaves behind afterwards. The subject of a counter is chosen by whoever sends the
  // request and the token of a session by whoever signs in, so a table nothing sweeps grows with every
  // attempt and every sign-in rather than with the installation.
  //
  // Everything the burst writes is already past its retention, so the sweep may take rows of it at any
  // moment. The burst and the reading of it are therefore one transaction, which no sweep can fall
  // inside: read row by row instead, the reading would measure where in the ten-second schedule the
  // burst landed rather than whether it was written.
  test('one burst of subjects grows the tables, and the sweep takes them back to the bound', async () => {
    resetRateLimits();
    const baseline = { counters: counterCount(), sessions: sessionCount() };

    const landed = writeBurst();
    assert.equal(
      landed.counters,
      WRITTEN_SUBJECTS,
      `the counter burst landed at ${landed.counters} of ${WRITTEN_SUBJECTS}`,
    );
    assert.equal(
      landed.sessions,
      WRITTEN_SUBJECTS,
      `the session burst landed at ${landed.sessions} of ${WRITTEN_SUBJECTS}`,
    );

    // Everything written is past its retention, so the sweep takes all of it and the table returns to
    // what it held before the burst: that is the bound the sweeps exist to keep.
    await untilSwept(() => expiredCounters() === 0 && expiredSessions() === 0, 'the sweep never took the burst back');
    process.stdout.write(
      `retention growth: counters ${baseline.counters} → ${landed.counters} → ${counterCount()}, ` +
        `sessions ${baseline.sessions} → ${landed.sessions} → ${sessionCount()}\n`,
    );
    assert.ok(
      Math.abs(counterCount() - baseline.counters) <= MEASUREMENT_SLACK,
      `the counter table did not return to its bound: ${counterCount()} against ${baseline.counters}`,
    );
    assert.ok(
      Math.abs(sessionCount() - baseline.sessions) <= MEASUREMENT_SLACK,
      `the session table did not return to its bound: ${sessionCount()} against ${baseline.sessions}`,
    );
  });
});

/** How many subjects one measurement writes, which is a burst rather than a single row. */
const WRITTEN_SUBJECTS = 25;

/**
 * How many rows the measurement tolerates beside the ones it wrote. Other suites share the stack and
 * add a handful of subjects of their own between the two readings, which is not what is measured: what
 * is measured is that a table does not keep the burst.
 */
const MEASUREMENT_SLACK = 40;

/** The addresses one measurement writes counters under, which no other suite uses. */
function growthCounterSubjects() {
  return Array.from({ length: WRITTEN_SUBJECTS }, (_, index) => newEmail(`growth-${index}`));
}

/** The tokens one measurement writes sessions under, which no other suite uses. */
function growthSessionTokens() {
  return Array.from({ length: WRITTEN_SUBJECTS }, (_, index) => `growth-${index}-${Date.now()}`);
}

/**
 * Writes one burst into both tables and reads back what it holds of it, as one transaction: the rows
 * are already past their retention, so a sweep that fell between the write and the reading would make
 * the measurement describe the schedule instead of the burst. Both counts are of the burst's own rows,
 * which no other suite writes.
 */
function writeBurst() {
  const subjects = growthCounterSubjects()
    .map((subject) => `'${subject}'`)
    .join(', ');
  const tokens = growthSessionTokens()
    .map((token) => `'${token}'`)
    .join(', ');
  const counted = sql(
    `INSERT INTO ${RATE_LIMIT_TABLE} (scope, subject, window_started_at, attempts)
     SELECT '${SIGN_IN_EMAIL_SCOPE}', subject, now() - interval '1 minute', 1
     FROM unnest(ARRAY[${subjects}]::text[]) AS subject;
     INSERT INTO ${SESSION_TABLE} (token, data, expiry)
     SELECT token, '{}'::bytea, now() - interval '1 minute'
     FROM unnest(ARRAY[${tokens}]::text[]) AS token;
     SELECT
       (SELECT count(*) FROM ${RATE_LIMIT_TABLE} WHERE subject IN (${subjects})) AS counters,
       (SELECT count(*) FROM ${SESSION_TABLE} WHERE token IN (${tokens})) AS sessions`,
  );
  // The two writes announce themselves with a command tag before the reading, so the row is the last
  // line of what the statement printed.
  const [counters, sessions] = counted.split('\n').at(-1).split('|').map(Number);
  return { counters, sessions };
}

function counterCount() {
  return Number(sql(`SELECT count(*) FROM ${RATE_LIMIT_TABLE}`));
}

function sessionCount() {
  return Number(sql(`SELECT count(*) FROM ${SESSION_TABLE}`));
}

function expiredCounters() {
  return Number(
    sql(
      `SELECT count(*) FROM ${RATE_LIMIT_TABLE}
       WHERE window_started_at < now() - interval '${COUNTER_RETENTION_HOURS} hours'`,
    ),
  );
}

function expiredSessions() {
  return Number(sql(`SELECT count(*) FROM ${SESSION_TABLE} WHERE expiry < now()`));
}

/** Writes one counter whose window began the stated number of hours ago. */
function writeCounter(scope, subject, hoursAgo) {
  sql(
    `INSERT INTO ${RATE_LIMIT_TABLE} (scope, subject, window_started_at, attempts)
     VALUES ('${scope}', '${subject}', now() - interval '${hoursAgo} hours', 1)`,
  );
}

function counterExists(scope, subject) {
  return sql(`SELECT 1 FROM ${RATE_LIMIT_TABLE} WHERE scope = '${scope}' AND subject = '${subject}'`) === '1';
}

function sessionExists(token) {
  return sql(`SELECT 1 FROM ${SESSION_TABLE} WHERE token = '${token}'`) === '1';
}

/** The value the session store keeps for a token: the base64url of its SHA-256. */
function hashedToken(token) {
  return createHash('sha256').update(token).digest('base64url');
}

/** Waits for a condition the sweep reaches on its own, or reports how long it waited. */
async function untilSwept(reached, complaint) {
  const deadline = Date.now() + SWEEP_PATIENCE_MS;
  while (!reached()) {
    if (Date.now() > deadline) throw new Error(`${complaint} within ${SWEEP_PATIENCE_MS} ms`);
    await delay(SWEEP_POLL_MS);
  }
}
