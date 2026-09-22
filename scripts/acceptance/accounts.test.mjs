// Registration, sign-in and session restoration, observed on the real HTTP boundary against the
// PostgreSQL the running service uses.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import {
  ARGON2ID_PHC_PATTERN,
  LONGEST_ACCEPTED_PASSWORD,
  PASSWORD_HASH_COLUMN,
  PHC_SALT_FIELD,
  SHORTEST_ACCEPTED_PASSWORD,
  SESSION_TOKEN_COLUMN,
} from './accounts.mjs';
import {
  call,
  compose,
  CURRENT_USER_PATH,
  newEmail,
  password,
  REGISTRATION_PATH,
  registerAccount,
  registrationRequest,
  resetRateLimits,
  sessionSetCookie,
  SIGN_IN_PATH,
  sql,
  waitForReady,
  wrongPassword,
} from './client.mjs';
import { SESSION_COOKIE_NAME } from '../service.mjs';

const SESSION_LIFETIME_HOURS = 12;
const SESSION_LIFETIME_TOLERANCE_HOURS = 0.02;
const MILLISECONDS_PER_HOUR = 3_600_000;

const NON_ASCII_PASSWORD = 'парольнадежный';
const SHARED_PASSWORD = 'sharedpasswordvalue';

// Each entry names why the policy refuses a password, and the password it refuses for that reason.
const REFUSED_PASSWORDS = new Map([
  ['is one code point short', 'a'.repeat(SHORTEST_ACCEPTED_PASSWORD - 1)],
  ['is one code point too long', 'a'.repeat(LONGEST_ACCEPTED_PASSWORD + 1)],
  ['contains a space', 'correct horse battery'],
  ['contains a tab', 'correcthorse\tbattery'],
  ['contains an ideographic space', 'correcthorse　battery'],
  ['contains a no-break space', 'correcthorse\u00a0battery'],
]);

const ACCEPTED_PASSWORDS = new Map([
  ['is exactly 12 code points', 'a'.repeat(SHORTEST_ACCEPTED_PASSWORD)],
  ['is exactly 128 code points', 'a'.repeat(LONGEST_ACCEPTED_PASSWORD)],
  ['is not ASCII', NON_ASCII_PASSWORD],
]);

before(() => waitForReady());

// Every suite shares one address, so each test starts with the rate limits untouched by the last.
beforeEach(resetRateLimits);

function countRows(table) {
  return Number(sql(`SELECT count(*) FROM ${table}`));
}

function countRowsWhere(table, condition) {
  return Number(sql(`SELECT count(*) FROM ${table} WHERE ${condition}`));
}

function storedPasswordHashes(emails) {
  const addresses = emails.map((email) => `'${email}'`).join(', ');
  return sql(`SELECT ${PASSWORD_HASH_COLUMN} FROM users WHERE email IN (${addresses})`).split('\n');
}

describe('registration establishes a session', () => {
  test('answers 201 with a snapshot that carries no session token', async () => {
    const { email, response } = await registerAccount('register');
    assert.equal(response.status, 201, response.text);
    assert.equal(response.json.user.email, email);
    assert.ok(response.json.user.id);
    assert.ok(response.json.csrf_token);
    assert.ok(response.json.server_time);
    assert.ok(response.json.session_expires_at);
    assert.ok(!response.text.includes(SESSION_COOKIE_NAME), 'the body names the session cookie');
  });

  test('issues an HttpOnly, Path=/, SameSite=Lax cookie lasting an absolute 12 hours', async () => {
    const { response } = await registerAccount('cookie');
    const issued = sessionSetCookie(response);
    assert.ok(issued, 'no session cookie was set');
    assert.match(issued, /HttpOnly/i);
    assert.match(issued, /Path=\//i);
    assert.match(issued, /SameSite=Lax/i);
    const elapsedHours =
      (new Date(response.json.session_expires_at) - new Date(response.json.server_time)) / MILLISECONDS_PER_HOUR;
    assert.ok(
      Math.abs(elapsedHours - SESSION_LIFETIME_HOURS) < SESSION_LIFETIME_TOLERANCE_HOURS,
      `lifetime was ${elapsedHours} hours`,
    );
  });

  test('answers with no-store', async () => {
    const { response } = await registerAccount('no-store');
    assert.equal(response.headers.get('cache-control'), 'no-store');
  });
});

describe('the canonical email is the identity', () => {
  test('trims and lowercases the whole address', async () => {
    const mixedCaseAddress = newEmail('Canonical').toUpperCase();
    const response = await call(REGISTRATION_PATH, registrationRequest(`  ${mixedCaseAddress}  `));
    assert.equal(response.status, 201, response.text);
    assert.equal(response.json.user.email, mixedCaseAddress.toLowerCase());
  });

  test('refuses a second registration of the same canonical address', async () => {
    const { email } = await registerAccount('duplicate');
    const repeat = await call(REGISTRATION_PATH, registrationRequest(email.toUpperCase()));
    assert.equal(repeat.status, 409, repeat.text);
    assert.equal(repeat.json.code, 'EMAIL_ALREADY_REGISTERED');
  });
});

describe('the password policy', () => {
  for (const [reason, refused] of REFUSED_PASSWORDS) {
    test(`refuses a password that ${reason}`, async () => {
      const response = await call(REGISTRATION_PATH, registrationRequest(newEmail('policy'), refused));
      assert.equal(response.status, 422, response.text);
    });
  }

  for (const [reason, accepted] of ACCEPTED_PASSWORDS) {
    test(`accepts a password that ${reason}`, async () => {
      const response = await call(REGISTRATION_PATH, registrationRequest(newEmail('policy'), accepted));
      assert.equal(response.status, 201, response.text);
    });
  }
});

describe('signing in', () => {
  test('answers 200 with a snapshot and a new CSRF token', async () => {
    const { email, response: registered } = await registerAccount('sign-in');
    const signedIn = await call(SIGN_IN_PATH, registrationRequest(email));
    assert.equal(signedIn.status, 200, signedIn.text);
    assert.equal(signedIn.json.user.email, email);
    assert.notEqual(signedIn.json.csrf_token, registered.json.csrf_token);
  });

  test('answers an unknown address and a wrong password identically', async () => {
    const { email } = await registerAccount('indistinguishable');
    const refusedPassword = await call(SIGN_IN_PATH, {
      method: 'POST',
      body: { email, password: wrongPassword },
    });
    const refusedAddress = await call(SIGN_IN_PATH, {
      method: 'POST',
      body: { email: newEmail('absent'), password },
    });
    assert.equal(refusedPassword.status, 401, refusedPassword.text);
    assert.equal(refusedAddress.status, 401, refusedAddress.text);
    assert.equal(refusedPassword.json.code, 'INVALID_CREDENTIALS');
    assert.equal(refusedAddress.json.code, 'INVALID_CREDENTIALS');
    assert.equal(refusedPassword.json.message, refusedAddress.json.message);
  });
});

describe('the session is the only way to name a user', () => {
  test('restores the caller through /me and refuses a caller without one', async () => {
    const { email, cookie, response } = await registerAccount('restore');
    const restored = await call(CURRENT_USER_PATH, { cookie });
    assert.equal(restored.status, 200, restored.text);
    assert.equal(restored.json.user.email, email);
    assert.equal(restored.json.user.id, response.json.user.id);

    const anonymous = await call(CURRENT_USER_PATH);
    assert.equal(anonymous.status, 401, anonymous.text);
  });

  test('does not renew the expiry when a live session is used', async () => {
    const { cookie, response } = await registerAccount('absolute');
    const restored = await call(CURRENT_USER_PATH, { cookie });
    assert.equal(restored.json.session_expires_at, response.json.session_expires_at);
  });

  test('gives two independent clients their own user and ignores a supplied id', async () => {
    const firstClient = await registerAccount('client-one');
    const secondClient = await registerAccount('client-two');
    const firstUser = await call(CURRENT_USER_PATH, { cookie: firstClient.cookie });
    const secondUser = await call(CURRENT_USER_PATH, { cookie: secondClient.cookie });
    assert.equal(firstUser.json.user.email, firstClient.email);
    assert.equal(secondUser.json.user.email, secondClient.email);

    const spoofed = await call(`${CURRENT_USER_PATH}?id=${firstUser.json.user.id}`, { cookie: secondClient.cookie });
    assert.equal(spoofed.json.user.email, secondClient.email, 'a query parameter selected another user');
  });
});

describe('what the database holds', () => {
  test('stores the session token only as its hash', async () => {
    const { cookie } = await registerAccount('hashed');
    const issuedToken = cookie.split('=')[1];
    const storedTokens = countRowsWhere('sessions', `${SESSION_TOKEN_COLUMN} = '${issuedToken}'`);
    assert.equal(storedTokens, 0, 'the raw session token is stored');
  });

  test('gives two accounts sharing a password different salts', async () => {
    const firstAccount = await registerAccount('salt', { password: SHARED_PASSWORD });
    const secondAccount = await registerAccount('salt', { password: SHARED_PASSWORD });
    const hashes = storedPasswordHashes([firstAccount.email, secondAccount.email]);
    assert.equal(hashes.length, 2);
    assert.notEqual(hashes[0], hashes[1], 'two accounts share a stored hash');

    const salts = hashes.map((hash) => hash.split('$')[PHC_SALT_FIELD]);
    assert.notEqual(salts[0], salts[1], 'two accounts share a salt');
    for (const hash of hashes) {
      assert.match(hash, ARGON2ID_PHC_PATTERN);
      assert.ok(!hash.includes(SHARED_PASSWORD), 'a stored hash contains the password');
    }
  });

  test('rolls the whole registration back when the session cannot be stored', async () => {
    const email = newEmail('rollback');
    const usersBefore = countRows('users');
    const sessionsBefore = countRows('sessions');
    sql('REVOKE INSERT ON sessions FROM carsharing_app');
    try {
      const refused = await call(REGISTRATION_PATH, registrationRequest(email));
      assert.equal(refused.status, 503, refused.text);
      assert.equal(countRowsWhere('users', `email = '${email}'`), 0, 'the user row survived');
      assert.equal(countRows('users'), usersBefore);
      assert.equal(countRows('sessions'), sessionsBefore);
    } finally {
      sql('GRANT INSERT ON sessions TO carsharing_app');
    }
    const retried = await call(REGISTRATION_PATH, registrationRequest(email));
    assert.equal(retried.status, 201, retried.text);
  });
});

describe('a session outlives the process that issued it', () => {
  test('survives an API restart with its original expiry', async () => {
    const { email, cookie, response } = await registerAccount('restart');
    compose('restart', 'api');
    await waitForReady();
    const restored = await call(CURRENT_USER_PATH, { cookie });
    assert.equal(restored.status, 200, restored.text);
    assert.equal(restored.json.user.email, email);
    assert.equal(restored.json.session_expires_at, response.json.session_expires_at);
  });
});

after(() => {
  // The suites share one database; leaving the grant in place keeps a later run from starting
  // against a service that cannot store sessions.
  sql('GRANT INSERT ON sessions TO carsharing_app');
});
