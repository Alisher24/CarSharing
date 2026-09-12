// The cost of verifying a password, observed against the running service. Argon2id is the most
// expensive thing a request can ask for, so what matters here is what the service refuses to do
// rather than what it computes.
import assert from 'node:assert/strict';
import { before, beforeEach, describe, test } from 'node:test';
import { ARGON2ID_PHC_PATTERN, PASSWORD_HASH_COLUMN, PHC_SALT_FIELD } from './accounts.mjs';
import {
  call,
  newEmail,
  registerAccount,
  resetRateLimits,
  SIGN_IN_PATH,
  signInRequest,
  sql,
  waitForReady,
  wrongPassword,
} from './client.mjs';

/**
 * The parameters recorded in the README beside the measurements. A stored hash carries the
 * parameters it was produced with, so comparing the two is how the deployed cost and the documented
 * cost are held together.
 */
const recordedHashingParameters = {
  memoryKiB: 19456,
  passes: 2,
  parallelism: 1,
};

/** How many hashes the instance admits at once, per the recorded configuration. */
const recordedConcurrentHashes = 2;

// Comfortably more than the ceiling, so the surplus has to be refused rather than queued. The
// address limit is high enough that these are refused for being surplus rather than for being too
// many attempts.
const REQUEST_BURST_MULTIPLIER = 6;

// Several rounds, comparing the fastest of each: a minimum is far less sensitive to scheduling
// noise than a mean, and a skipped hash would show up as a floor an order of magnitude lower.
const TIMING_ROUNDS = 5;
const MINIMUM_KNOWN_ADDRESS_SPEED_RATIO = 0.5;

const ACCEPTED_SIGN_IN_STATUS = 200;
const REFUSED_SIGN_IN_STATUS = 401;
const OVERLOADED_STATUS = 503;
const SERVICE_UNAVAILABLE_CODE = 'SERVICE_UNAVAILABLE';

const SHARED_PASSWORD = 'sharedpasswordvalue';

/** How many sign-ins the suite asks for at once, which is comfortably more than the ceiling. */
const REQUEST_BURST_SIZE = recordedConcurrentHashes * REQUEST_BURST_MULTIPLIER;

before(waitForReady);
beforeEach(resetRateLimits);

function assertRecordedParameters(storedHash) {
  const match = storedHash.match(ARGON2ID_PHC_PATTERN);
  assert.ok(match, `the stored hash is not a recognisable Argon2id PHC string: ${storedHash}`);
  assert.equal(
    Number(match[1]),
    recordedHashingParameters.memoryKiB,
    'deployed memory differs from the recorded value',
  );
  assert.equal(Number(match[2]), recordedHashingParameters.passes, 'deployed passes differ from the recorded value');
  assert.equal(
    Number(match[3]),
    recordedHashingParameters.parallelism,
    'deployed parallelism differs from the recorded value',
  );
}

/** Asks for more sign-ins at once than the instance can hash, and returns every answer. */
function askForMoreSignInsThanTheCeilingAllows(email) {
  return Promise.all(Array.from({ length: REQUEST_BURST_SIZE }, () => call(SIGN_IN_PATH, signInRequest(email))));
}

/** The salt each account was hashed with, read back from the stored PHC strings. */
function storedSalts(firstEmail, secondEmail) {
  const query =
    `SELECT split_part(${PASSWORD_HASH_COLUMN}, '$', ${PHC_SALT_FIELD}) FROM users` +
    ` WHERE email IN ('${firstEmail}', '${secondEmail}')`;
  return sql(query).split('\n');
}

describe('the deployed hashing parameters are the recorded ones', () => {
  test('a freshly stored hash carries the parameters the README records', async () => {
    const { email } = await registerAccount('parameters');
    assertRecordedParameters(sql(`SELECT ${PASSWORD_HASH_COLUMN} FROM users WHERE email = '${email}'`));
  });

  test('every account is hashed with its own salt', async () => {
    const firstAccount = await registerAccount('own-salt', { password: SHARED_PASSWORD });
    const secondAccount = await registerAccount('own-salt', { password: SHARED_PASSWORD });
    const salts = storedSalts(firstAccount.email, secondAccount.email);
    assert.equal(salts.length, 2);
    assert.notEqual(salts[0], salts[1]);
  });
});

describe('an instance admits only its ceiling of concurrent hashes', () => {
  test('refuses the surplus with 503 instead of queueing it', async () => {
    const { email } = await registerAccount('ceiling');
    resetRateLimits();

    const answers = await askForMoreSignInsThanTheCeilingAllows(email);
    const statuses = answers.map((answer) => answer.status);
    const unavailable = answers.filter((answer) => answer.status === OVERLOADED_STATUS);
    const succeeded = statuses.filter((status) => status === ACCEPTED_SIGN_IN_STATUS).length;

    assert.ok(
      unavailable.length > 0,
      `no request was refused while ${REQUEST_BURST_SIZE} hashes were asked for at once: ${statuses.join(',')}`,
    );
    assert.ok(succeeded > 0, `every request was refused: ${statuses.join(',')}`);
    for (const refused of unavailable) {
      assert.equal(refused.json.code, SERVICE_UNAVAILABLE_CODE, refused.text);
    }
    // Nothing may answer with anything else: a queued request would eventually appear as a
    // timeout or a gateway error rather than as an honest refusal.
    for (const status of statuses) {
      assert.ok(
        [ACCEPTED_SIGN_IN_STATUS, OVERLOADED_STATUS].includes(status),
        `an unexpected status appeared under pressure: ${status}`,
      );
    }
  });

  test('recovers as soon as the pressure is gone', async () => {
    const { email } = await registerAccount('recovers');
    resetRateLimits();
    await askForMoreSignInsThanTheCeilingAllows(email);
    const afterwards = await call(SIGN_IN_PATH, signInRequest(email));
    assert.equal(afterwards.status, ACCEPTED_SIGN_IN_STATUS, `the instance did not recover: ${afterwards.text}`);
  });
});

describe('an unknown address costs the same work as a known one', () => {
  test('does not answer an unregistered address markedly faster', async () => {
    const { email } = await registerAccount('timing');
    resetRateLimits();
    const unknownAddress = newEmail('never-registered');

    const fastestMilliseconds = { known: Infinity, unknown: Infinity };
    for (let round = 0; round < TIMING_ROUNDS; round += 1) {
      for (const [addressKind, address] of [
        ['known', email],
        ['unknown', unknownAddress],
      ]) {
        resetRateLimits();
        const startedAt = performance.now();
        const answer = await call(SIGN_IN_PATH, {
          method: 'POST',
          body: { email: address, password: wrongPassword },
        });
        assert.equal(answer.status, REFUSED_SIGN_IN_STATUS, answer.text);
        fastestMilliseconds[addressKind] = Math.min(fastestMilliseconds[addressKind], performance.now() - startedAt);
      }
    }
    assert.ok(
      fastestMilliseconds.unknown > fastestMilliseconds.known * MINIMUM_KNOWN_ADDRESS_SPEED_RATIO,
      `an unknown address answered in ${fastestMilliseconds.unknown.toFixed(1)}ms against ` +
        `${fastestMilliseconds.known.toFixed(1)}ms for a known one, which suggests the hash was skipped`,
    );
  });
});
