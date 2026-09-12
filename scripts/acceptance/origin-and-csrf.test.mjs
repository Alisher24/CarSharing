// Which page may act, and with what token. Observed on the real HTTP boundary, because the point
// of both rules is what the running service refuses before anything is created.
import assert from 'node:assert/strict';
import { before, describe, test } from 'node:test';
import { call, newEmail, password, registerAccount, sessionCookie, waitForReady } from './client.mjs';

const foreignOrigin = 'http://attacker.example';

before(waitForReady);

describe('a mutation from an origin this application does not allow', () => {
  for (const [name, path, hasBody] of [
    ['registration', '/api/v1/auth/register', true],
    ['sign-in', '/api/v1/auth/login', true],
    ['sign-out', '/api/v1/auth/logout', false],
  ]) {
    test(`refuses ${name} from a foreign origin without a side effect`, async () => {
      const email = newEmail('foreign');
      const refused = await call(path, {
        method: 'POST',
        body: hasBody ? { email, password } : undefined,
        origin: foreignOrigin,
      });
      assert.equal(refused.status, 403, refused.text);
      assert.equal(refused.json.code, 'ORIGIN_NOT_ALLOWED');
      assert.deepEqual(refused.setCookie, [], 'a refused request touched the cookie');
      if (hasBody) {
        // The address is still free, so the refused request created no account.
        const after = await call('/api/v1/auth/register', { method: 'POST', body: { email, password } });
        assert.equal(after.status, 201, 'the refused request created an account');
      }
    });

    test(`refuses ${name} with no Origin at all`, async () => {
      const refused = await call(path, {
        method: 'POST',
        body: hasBody ? { email: newEmail('absent-origin'), password } : undefined,
        omitOrigin: true,
      });
      assert.equal(refused.status, 403, refused.text);
      assert.equal(refused.json.code, 'ORIGIN_NOT_ALLOWED');
      assert.deepEqual(refused.setCookie, []);
    });
  }

  test('lets an allowed origin through', async () => {
    const { response } = await registerAccount('allowed');
    assert.equal(response.status, 201, response.text);
  });
});

describe('the CSRF token is bound to one session', () => {
  test('is issued inside the snapshot', async () => {
    const { response } = await registerAccount('csrf-issued');
    assert.equal(typeof response.json.csrf_token, 'string');
    assert.ok(response.json.csrf_token.length > 20, 'the CSRF token is too short to be unguessable');
  });

  test('refuses an authenticated mutation that carries none', async () => {
    const { cookie } = await registerAccount('csrf-absent');
    const refused = await call('/api/v1/auth/logout', { method: 'POST', cookie });
    assert.equal(refused.status, 403, refused.text);
    assert.equal(refused.json.code, 'CSRF_INVALID');
    assert.equal((await call('/api/v1/me', { cookie })).status, 200, 'the refusal revoked the session');
  });

  test('refuses an authenticated mutation that carries the wrong one', async () => {
    const { cookie } = await registerAccount('csrf-wrong');
    const refused = await call('/api/v1/auth/logout', {
      method: 'POST',
      cookie,
      csrfToken: 'not-the-token-this-session-was-issued',
    });
    assert.equal(refused.status, 403, refused.text);
    assert.equal(refused.json.code, 'CSRF_INVALID');
    assert.equal((await call('/api/v1/me', { cookie })).status, 200, 'the refusal revoked the session');
  });

  test('accepts the token its own session was issued', async () => {
    const { cookie, csrfToken } = await registerAccount('csrf-right');
    const accepted = await call('/api/v1/auth/logout', { method: 'POST', cookie, csrfToken });
    assert.equal(accepted.status, 204, accepted.text);
  });
});

describe('replacing a session retires the token it issued', () => {
  test('a new session never accepts the previous session\'s CSRF token', async () => {
    const { email, cookie, csrfToken } = await registerAccount('csrf-replaced');
    const replaced = await call('/api/v1/auth/login', {
      method: 'POST',
      body: { email, password },
      cookie,
    });
    assert.equal(replaced.status, 200, replaced.text);
    const newCookie = sessionCookie(replaced);
    assert.notEqual(replaced.json.csrf_token, csrfToken, 'the replacement reused the old CSRF token');

    const stale = await call('/api/v1/auth/logout', { method: 'POST', cookie: newCookie, csrfToken });
    assert.equal(stale.status, 403, stale.text);
    assert.equal(stale.json.code, 'CSRF_INVALID');

    assert.equal((await call('/api/v1/me', { cookie })).status, 401, 'the replaced session still authorizes');
  });
});
