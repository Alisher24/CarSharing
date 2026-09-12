// The cost of verifying a password, observed against the running service. Argon2id is the most
// expensive thing a request can ask for, so what matters here is what the service refuses to do
// rather than what it computes.
import assert from 'node:assert/strict';
import { before, beforeEach, describe, test } from 'node:test';
import { call, newEmail, password, registerAccount, resetRateLimits, sql, waitForReady } from './client.mjs';

before(waitForReady);
beforeEach(resetRateLimits);

/**
 * The parameters recorded in the README beside the measurements. A stored hash carries the
 * parameters it was produced with, so comparing the two is how the deployed cost and the documented
 * cost are held together.
 */
const recorded = { memoryKiB: 19456, passes: 2, parallelism: 1 };

/** How many hashes the instance admits at once, per the recorded configuration. */
const recordedConcurrent = 2;

describe('the deployed hashing parameters are the recorded ones', () => {
  test('a freshly stored hash carries the parameters the README records', async () => {
    const { email } = await registerAccount('parameters');
    const stored = sql(`SELECT password_hash FROM users WHERE email = '${email}'`);
    const match = stored.match(/^\$argon2id\$v=19\$m=(\d+),t=(\d+),p=(\d+)\$/);
    assert.ok(match, `the stored hash is not a recognisable Argon2id PHC string: ${stored}`);
    assert.equal(Number(match[1]), recorded.memoryKiB, 'deployed memory differs from the recorded value');
    assert.equal(Number(match[2]), recorded.passes, 'deployed passes differ from the recorded value');
    assert.equal(Number(match[3]), recorded.parallelism, 'deployed parallelism differs from the recorded value');
  });

  test('every account is hashed with its own salt', async () => {
    const shared = 'sharedpasswordvalue';
    const one = await registerAccount('own-salt', { password: shared });
    const two = await registerAccount('own-salt', { password: shared });
    const salts = sql(
      `SELECT split_part(password_hash, '$', 5) FROM users WHERE email IN ('${one.email}', '${two.email}')`,
    ).split('\n');
    assert.equal(salts.length, 2);
    assert.notEqual(salts[0], salts[1]);
  });
});

describe('an instance admits only its ceiling of concurrent hashes', () => {
  test('refuses the surplus with 503 instead of queueing it', async () => {
    const { email } = await registerAccount('ceiling');
    resetRateLimits();

    // Comfortably more than the ceiling, all in flight at once. The address limit is high enough
    // that these are refused for being surplus rather than for being too many attempts.
    const inFlight = recordedConcurrent * 6;
    const answers = await Promise.all(
      Array.from({ length: inFlight }, () =>
        call('/api/v1/auth/login', { method: 'POST', body: { email, password } })),
    );
    const statuses = answers.map((answer) => answer.status);
    const unavailable = answers.filter((answer) => answer.status === 503);
    const succeeded = statuses.filter((status) => status === 200).length;

    assert.ok(
      unavailable.length > 0,
      `no request was refused while ${inFlight} hashes were asked for at once: ${statuses.join(',')}`,
    );
    assert.ok(succeeded > 0, `every request was refused: ${statuses.join(',')}`);
    for (const refused of unavailable) {
      assert.equal(refused.json.code, 'SERVICE_UNAVAILABLE', refused.text);
    }
    // Nothing may answer with anything else: a queued request would eventually appear as a
    // timeout or a gateway error rather than as an honest refusal.
    for (const status of statuses) {
      assert.ok([200, 503].includes(status), `an unexpected status appeared under pressure: ${status}`);
    }
  });

  test('recovers as soon as the pressure is gone', async () => {
    const { email } = await registerAccount('recovers');
    resetRateLimits();
    await Promise.all(
      Array.from({ length: recordedConcurrent * 6 }, () =>
        call('/api/v1/auth/login', { method: 'POST', body: { email, password } })),
    );
    const afterwards = await call('/api/v1/auth/login', { method: 'POST', body: { email, password } });
    assert.equal(afterwards.status, 200, `the instance did not recover: ${afterwards.text}`);
  });
});

describe('an unknown address costs the same work as a known one', () => {
  test('does not answer an unregistered address markedly faster', async () => {
    const { email } = await registerAccount('timing');
    resetRateLimits();
    const unknown = newEmail('never-registered');

    // Several rounds, comparing the fastest of each: a minimum is far less sensitive to scheduling
    // noise than a mean, and a skipped hash would show up as a floor an order of magnitude lower.
    const fastest = { known: Infinity, unknown: Infinity };
    for (let round = 0; round < 5; round += 1) {
      for (const [label, address] of [['known', email], ['unknown', unknown]]) {
        resetRateLimits();
        const started = performance.now();
        const answer = await call('/api/v1/auth/login', {
          method: 'POST',
          body: { email: address, password: 'wrongpasswordvalue' },
        });
        assert.equal(answer.status, 401, answer.text);
        fastest[label] = Math.min(fastest[label], performance.now() - started);
      }
    }
    assert.ok(
      fastest.unknown > fastest.known / 2,
      `an unknown address answered in ${fastest.unknown.toFixed(1)}ms against ${fastest.known.toFixed(1)}ms `
        + 'for a known one, which suggests the hash was skipped',
    );
  });
});
