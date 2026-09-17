// The secrets of an assembled stack, after a scenario has been driven through every service that holds
// one: what each process was given must not appear in what the stack says about itself.
//
// The check is a search of the logs rather than a reading of them, because a secret is not a value the
// service is supposed to print in one particular line: any occurrence at all is the failure. The same
// search covers the answers a caller can ask for, so a secret that reached a body rather than a log is
// caught by the same pass.
//
// The scenario is driven deliberately before the reading: a stack that has served nothing has logged
// nothing, and a search of an empty log proves nothing about a service that has been working.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { after, before, describe, test } from 'node:test';
import { call, compose, resetRateLimits, serviceOrigin, sql, waitForReady } from './client.mjs';
import { DELIVERY_PATH, internalCall } from './mail.mjs';
import { endSuiteReservations, newAccount, newCommandKey, reserve, restoreScenario } from './reservations.mjs';
import { SECRETS_DIRECTORY } from '../setup.mjs';

/**
 * The one file of that directory that is not a credential of a running service: the account of the
 * demonstration is a value a person signs in with, and a person may write it down.
 */
const NOT_A_SERVICE_CREDENTIAL = ['demo_user_password'];

/**
 * The shortest value this check searches for. A service credential is a name's worth of random bytes,
 * so a shorter value would be one that other text can contain by chance.
 */
const MINIMUM_SECRET_LENGTH = 16;

/**
 * How much of every service's log is searched, which is the whole of what the stack wrote in this
 * session and several times the lines one scenario produces.
 */
const LOG_TAIL_LINES = '2000';

before(async () => {
  await waitForReady();
});

// The scenario below reserves a vehicle, and the demonstration refuses to be put back while a rental
// of a person's stands on one of its vehicles, so the suite releases what it took.
after(async () => {
  await endSuiteReservations();
  restoreScenario();
});

describe('the secrets of the installation stay out of what the stack publishes', () => {
  test('no credential of a service appears in the logs or in any answer after a scenario', async () => {
    resetRateLimits();
    const secrets = readSecrets();
    assert.ok(secrets.length > 0, 'the installation holds no secret to check');

    // A scenario that puts every one of them to work: a registration and a reservation touch the
    // database credentials, a page of notifications is signed with the cursor key, an internal call
    // presents a capability, and a refused sign-in reads the credentials of the account store.
    const account = await newAccount('secrets');
    const created = await reserve(freeVehicle(), newCommandKey(), account);
    assert.equal(created.status, 201, created.text);
    const notifications = await call('/api/v1/me/notifications?limit=1', { cookie: account.cookie });
    assert.equal(notifications.status, 200, notifications.text);
    const refused = internalCall('POST', DELIVERY_PATH, {
      body: JSON.stringify({ delivery_key: 'secrets-check', recipient: account.email, subject: 'x', text: 'y' }),
      token: 'not-the-delivery-token',
    });
    assert.equal(refused.status, 401, `an internal call with a wrong capability answered ${refused.status}`);
    await refusedSignIn(account.email);

    // The whole log is the corpus, not the tail of one service: a secret printed once, an hour ago, is
    // still a secret that was written to a file somebody reads.
    const corpus = `${compose('logs', '--no-color', '--tail', LOG_TAIL_LINES)}\n${refused.text}\n${notifications.text}`;
    const leaked = secrets.filter((secret) => corpus.includes(secret.value));
    assert.deepEqual(
      leaked.map((secret) => secret.name),
      [],
      `the stack published ${leaked.length} of its own credentials`,
    );

    process.stdout.write(
      `secrets: ${secrets.length} credentials of the installation, ${corpus.length} characters of logs and ` +
        `answers, none of them published\n`,
    );
  });

  test('the mail stub refuses a delivery presented with another capability and names no secret', async () => {
    const answer = internalCall('POST', DELIVERY_PATH, {
      body: JSON.stringify({
        delivery_key: 'secrets-refusal',
        recipient: 'nobody@example.test',
        subject: 'x',
        text: 'y',
      }),
      token: 'not-the-delivery-token',
    });
    assert.equal(answer.status, 401, answer.text);

    for (const secret of readSecrets()) {
      assert.ok(!answer.text.includes(secret.value), `the refusal named the credential ${secret.name}`);
    }
    assert.ok(!/file|path|token_file|secret/i.test(answer.text), `the refusal names a setting: ${answer.text}`);
  });
});

/** Every credential the installation holds, as the name it is filed under and the value itself. */
function readSecrets() {
  return readdirSync(SECRETS_DIRECTORY)
    .filter((name) => !NOT_A_SERVICE_CREDENTIAL.includes(name))
    .map((name) => ({ name, value: readFileSync(join(SECRETS_DIRECTORY, name), 'utf8').trim() }))
    .filter((secret) => secret.value.length >= MINIMUM_SECRET_LENGTH);
}

/** One refused sign-in, which is what makes the service read the credentials it was configured with. */
async function refusedSignIn(email) {
  const answer = await fetch(`${serviceOrigin}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Origin: serviceOrigin },
    body: JSON.stringify({ email, password: 'not-the-password' }),
  });
  assert.equal(answer.status, 401, `a refused sign-in answered ${answer.status}`);
}

/** A vehicle the demonstration has free, which the scenario of this check reserves. */
function freeVehicle() {
  return sql(
    `SELECT id FROM vehicles WHERE id NOT IN (SELECT vehicle_id FROM rentals WHERE ended_at IS NULL)
     ORDER BY id LIMIT 1`,
  );
}
