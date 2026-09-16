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

before(waitForReady);

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
    // expiry. The expired one names a token nobody holds, which is what a session left behind by a
    // browser that never came back looks like.
    sql(
      `INSERT INTO ${SESSION_TABLE} (token, data, expiry)
       VALUES ('${expired}', '{}'::bytea, now() - interval '1 minute')`,
    );
    assert.equal(sessionExists(expired), true, 'the expired session was not written');

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
});

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
