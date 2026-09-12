// Signing out ends one session and leaves the account. Observed on the real HTTP boundary, because
// what matters is which sessions the running service still honours afterwards.
import assert from 'node:assert/strict';
import { before, describe, test } from 'node:test';
import { call, password, registerAccount, sessionCookie, sessionSetCookie, waitForReady } from './client.mjs';

before(waitForReady);

/** Signs the same account in again without presenting the first session, as a second device would. */
async function secondDevice(email) {
  const response = await call('/api/v1/auth/login', { method: 'POST', body: { email, password } });
  assert.equal(response.status, 200, response.text);
  return { cookie: sessionCookie(response), csrfToken: response.json.csrf_token };
}

describe('signing out ends exactly this session', () => {
  test('answers 204 with no body and a cookie that clears the browser', async () => {
    const { cookie, csrfToken } = await registerAccount('sign-out');
    const out = await call('/api/v1/auth/logout', { method: 'POST', cookie, csrfToken });
    assert.equal(out.status, 204, out.text);
    assert.equal(out.text, '', 'the 204 carried a body');
    const cleared = sessionSetCookie(out);
    assert.ok(cleared, 'sign-out set no clearing cookie');
    assert.match(cleared, /Max-Age=0|Expires=Thu, 01 Jan 1970/i);
    // The clearing cookie must carry the attributes of the one it replaces, or the browser keeps
    // the original and the person stays signed in on screen.
    assert.match(cleared, /HttpOnly/i);
    assert.match(cleared, /Path=\//i);
    assert.match(cleared, /SameSite=Lax/i);
  });

  test('leaves the revoked session authorizing nothing', async () => {
    const { cookie, csrfToken } = await registerAccount('revoked');
    await call('/api/v1/auth/logout', { method: 'POST', cookie, csrfToken });
    assert.equal((await call('/api/v1/me', { cookie })).status, 401);
  });

  test('keeps an independent session of the same user working', async () => {
    const { email, cookie, csrfToken } = await registerAccount('other-device');
    const other = await secondDevice(email);
    assert.notEqual(other.cookie, cookie, 'the second sign-in reused the first session');

    await call('/api/v1/auth/logout', { method: 'POST', cookie, csrfToken });

    const survivor = await call('/api/v1/me', { cookie: other.cookie });
    assert.equal(survivor.status, 200, survivor.text);
    assert.equal(survivor.json.user.email, email);
  });
});

describe('signing out is always safe to repeat', () => {
  test('answers 204 when the session was already revoked', async () => {
    const { cookie, csrfToken } = await registerAccount('repeat');
    await call('/api/v1/auth/logout', { method: 'POST', cookie, csrfToken });
    const again = await call('/api/v1/auth/logout', { method: 'POST', cookie, csrfToken });
    assert.equal(again.status, 204, again.text);
    assert.ok(sessionSetCookie(again), 'the repeat cleared no cookie');
  });

  test('answers 204 when the request carries no session at all', async () => {
    const none = await call('/api/v1/auth/logout', { method: 'POST' });
    assert.equal(none.status, 204, none.text);
    assert.ok(sessionSetCookie(none), 'the request cleared no cookie');
  });

  test('answers 204 when the token names no session', async () => {
    const unknown = await call('/api/v1/auth/logout', {
      method: 'POST',
      cookie: 'carsharing_session=a-token-that-names-no-session',
    });
    assert.equal(unknown.status, 204, unknown.text);
  });
});

describe('the origin is checked before the session is', () => {
  test('refuses a foreign origin even when there is no session to end', async () => {
    const refused = await call('/api/v1/auth/logout', { method: 'POST', origin: 'http://attacker.example' });
    assert.equal(refused.status, 403, refused.text);
    assert.equal(refused.json.code, 'ORIGIN_NOT_ALLOWED');
    assert.deepEqual(refused.setCookie, [], 'the refusal touched the cookie');
  });

  test('does not let a foreign page sign a live session out', async () => {
    const { cookie, csrfToken } = await registerAccount('foreign-logout');
    const refused = await call('/api/v1/auth/logout', {
      method: 'POST',
      cookie,
      csrfToken,
      origin: 'http://attacker.example',
    });
    assert.equal(refused.status, 403, refused.text);
    assert.equal((await call('/api/v1/me', { cookie })).status, 200, 'a foreign page revoked the session');
  });
});
