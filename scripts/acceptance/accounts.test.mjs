// Registration, sign-in and session restoration, observed on the real HTTP boundary against the
// PostgreSQL the running service uses.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import { call, compose, newEmail, password, registerAccount, resetRateLimits, sessionSetCookie, sql, waitForReady } from './client.mjs';

before(waitForReady);

// Every suite shares one address, so each test starts with the limits of Q11 untouched by the last.
beforeEach(resetRateLimits);

describe('registration establishes a session', () => {
  test('answers 201 with a snapshot that carries no session token', async () => {
    const { email, response } = await registerAccount('register');
    assert.equal(response.status, 201, response.text);
    assert.equal(response.json.user.email, email);
    assert.ok(response.json.user.id);
    assert.ok(response.json.csrf_token);
    assert.ok(response.json.server_time);
    assert.ok(response.json.session_expires_at);
    assert.ok(!response.text.includes('carsharing_session'), 'the body names the session cookie');
  });

  test('issues an HttpOnly, Path=/, SameSite=Lax cookie lasting an absolute 12 hours', async () => {
    const { response } = await registerAccount('cookie');
    const issued = sessionSetCookie(response);
    assert.ok(issued, 'no session cookie was set');
    assert.match(issued, /HttpOnly/i);
    assert.match(issued, /Path=\//i);
    assert.match(issued, /SameSite=Lax/i);
    const hours = (new Date(response.json.session_expires_at) - new Date(response.json.server_time)) / 3_600_000;
    assert.ok(Math.abs(hours - 12) < 0.02, `lifetime was ${hours} hours`);
  });

  test('answers with no-store', async () => {
    const { response } = await registerAccount('no-store');
    assert.equal(response.headers.get('cache-control'), 'no-store');
  });
});

describe('the canonical email is the identity', () => {
  test('trims and lowercases the whole address', async () => {
    const mixed = newEmail('Canonical').toUpperCase();
    const response = await call('/api/v1/auth/register', {
      method: 'POST',
      body: { email: `  ${mixed}  `, password },
    });
    assert.equal(response.status, 201, response.text);
    assert.equal(response.json.user.email, mixed.toLowerCase());
  });

  test('refuses a second registration of the same canonical address', async () => {
    const { email } = await registerAccount('duplicate');
    const repeat = await call('/api/v1/auth/register', {
      method: 'POST',
      body: { email: email.toUpperCase(), password },
    });
    assert.equal(repeat.status, 409, repeat.text);
    assert.equal(repeat.json.code, 'EMAIL_ALREADY_REGISTERED');
  });
});

describe('the password policy', () => {
  for (const [reason, refused] of Object.entries({
    'is one code point short': 'a'.repeat(11),
    'is one code point too long': 'a'.repeat(129),
    'contains a space': 'correct horse battery',
    'contains a tab': 'correcthorse\tbattery',
    'contains an ideographic space': 'correcthorse　battery',
    'contains a no-break space': 'correcthorse battery',
  })) {
    test(`refuses a password that ${reason}`, async () => {
      const response = await call('/api/v1/auth/register', {
        method: 'POST',
        body: { email: newEmail('policy'), password: refused },
      });
      assert.equal(response.status, 422, response.text);
    });
  }

  for (const [reason, accepted] of Object.entries({
    'is exactly 12 code points': 'a'.repeat(12),
    'is exactly 128 code points': 'a'.repeat(128),
    'is not ASCII': 'парольнадежный',
  })) {
    test(`accepts a password that ${reason}`, async () => {
      const response = await call('/api/v1/auth/register', {
        method: 'POST',
        body: { email: newEmail('policy'), password: accepted },
      });
      assert.equal(response.status, 201, response.text);
    });
  }
});

describe('signing in', () => {
  test('answers 200 with a snapshot and a new CSRF token', async () => {
    const { email, response: registered } = await registerAccount('sign-in');
    const signedIn = await call('/api/v1/auth/login', { method: 'POST', body: { email, password } });
    assert.equal(signedIn.status, 200, signedIn.text);
    assert.equal(signedIn.json.user.email, email);
    assert.notEqual(signedIn.json.csrf_token, registered.json.csrf_token);
  });

  test('answers an unknown address and a wrong password identically', async () => {
    const { email } = await registerAccount('indistinguishable');
    const wrongPassword = await call('/api/v1/auth/login', {
      method: 'POST',
      body: { email, password: 'wrongpasswordvalue' },
    });
    const unknownEmail = await call('/api/v1/auth/login', {
      method: 'POST',
      body: { email: newEmail('absent'), password },
    });
    assert.equal(wrongPassword.status, 401, wrongPassword.text);
    assert.equal(unknownEmail.status, 401, unknownEmail.text);
    assert.equal(wrongPassword.json.code, 'INVALID_CREDENTIALS');
    assert.equal(unknownEmail.json.code, 'INVALID_CREDENTIALS');
    assert.equal(wrongPassword.json.message, unknownEmail.json.message);
  });
});

describe('the session is the only way to name a user', () => {
  test('restores the caller through /me and refuses a caller without one', async () => {
    const { email, cookie, response } = await registerAccount('restore');
    const restored = await call('/api/v1/me', { cookie });
    assert.equal(restored.status, 200, restored.text);
    assert.equal(restored.json.user.email, email);
    assert.equal(restored.json.user.id, response.json.user.id);

    const anonymous = await call('/api/v1/me');
    assert.equal(anonymous.status, 401, anonymous.text);
  });

  test('does not renew the expiry when a live session is used', async () => {
    const { cookie, response } = await registerAccount('absolute');
    const restored = await call('/api/v1/me', { cookie });
    assert.equal(restored.json.session_expires_at, response.json.session_expires_at);
  });

  test('gives two independent clients their own user and ignores a supplied id', async () => {
    const one = await registerAccount('client-one');
    const two = await registerAccount('client-two');
    const first = await call('/api/v1/me', { cookie: one.cookie });
    const second = await call('/api/v1/me', { cookie: two.cookie });
    assert.equal(first.json.user.email, one.email);
    assert.equal(second.json.user.email, two.email);

    const spoofed = await call(`/api/v1/me?id=${first.json.user.id}`, { cookie: two.cookie });
    assert.equal(spoofed.json.user.email, two.email, 'a query parameter selected another user');
  });
});

describe('what the database holds', () => {
  test('stores the session token only as its hash', async () => {
    const { cookie } = await registerAccount('hashed');
    const token = cookie.split('=')[1];
    const stored = sql(`SELECT count(*) FROM sessions WHERE token = '${token}'`);
    assert.equal(stored, '0', 'the raw session token is stored');
  });

  test('gives two accounts sharing a password different salts', async () => {
    const shared = 'sharedpasswordvalue';
    const one = await registerAccount('salt', { password: shared });
    const two = await registerAccount('salt', { password: shared });
    const hashes = sql(
      `SELECT password_hash FROM users WHERE email IN ('${one.email}', '${two.email}')`,
    ).split('\n');
    assert.equal(hashes.length, 2);
    assert.notEqual(hashes[0], hashes[1], 'two accounts share a stored hash');
    assert.notEqual(hashes[0].split('$')[4], hashes[1].split('$')[4], 'two accounts share a salt');
    for (const hash of hashes) {
      assert.match(hash, /^\$argon2id\$v=19\$m=\d+,t=\d+,p=\d+\$/);
      assert.ok(!hash.includes(shared), 'a stored hash contains the password');
    }
  });

  test('rolls the whole registration back when the session cannot be stored', async () => {
    const email = newEmail('rollback');
    const users = Number(sql('SELECT count(*) FROM users'));
    const sessions = Number(sql('SELECT count(*) FROM sessions'));
    sql('REVOKE INSERT ON sessions FROM carsharing_app');
    try {
      const refused = await call('/api/v1/auth/register', { method: 'POST', body: { email, password } });
      assert.equal(refused.status, 503, refused.text);
      assert.equal(sql(`SELECT count(*) FROM users WHERE email = '${email}'`), '0', 'the user row survived');
      assert.equal(Number(sql('SELECT count(*) FROM users')), users);
      assert.equal(Number(sql('SELECT count(*) FROM sessions')), sessions);
    } finally {
      sql('GRANT INSERT ON sessions TO carsharing_app');
    }
    const retried = await call('/api/v1/auth/register', { method: 'POST', body: { email, password } });
    assert.equal(retried.status, 201, retried.text);
  });
});

describe('a session outlives the process that issued it', () => {
  test('survives an API restart with its original expiry', async () => {
    const { email, cookie, response } = await registerAccount('restart');
    compose('restart', 'api');
    await waitForReady();
    const restored = await call('/api/v1/me', { cookie });
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
