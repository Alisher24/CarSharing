// Signing out ends one session and leaves the account. Observed on the real HTTP boundary, because
// what matters is which sessions the running service still honours afterwards.
import assert from 'node:assert/strict';
import { before, beforeEach, describe, test } from 'node:test';
import {
  call,
  CURRENT_USER_PATH,
  foreignOrigin,
  registerAccount,
  resetRateLimits,
  sessionSetCookie,
  signInFromSecondDevice,
  SIGN_OUT_PATH,
  waitForReady,
} from './client.mjs';
import { SESSION_COOKIE_NAME } from '../service.mjs';

const SIGNED_OUT_STATUS = 204;
const REFUSED_STATUS = 403;
const ORIGIN_NOT_ALLOWED_CODE = 'ORIGIN_NOT_ALLOWED';
const CLEARED_COOKIE_PATTERN = /Max-Age=0|Expires=Thu, 01 Jan 1970/i;
const UNKNOWN_SESSION_COOKIE = `${SESSION_COOKIE_NAME}=a-token-that-names-no-session`;

before(waitForReady);

// Every suite shares one address, so each test starts with the rate limits untouched by the last.
// The hook is given the running context as its first argument, so the helper is called from a
// function of its own: passing it directly would hand the context to it as the address to restore.
beforeEach(() => resetRateLimits());

describe('signing out ends exactly this session', () => {
  test('answers 204 with no body and a cookie that clears the browser', async () => {
    const { cookie, csrfToken } = await registerAccount('sign-out');
    const signedOut = await call(SIGN_OUT_PATH, { method: 'POST', cookie, csrfToken });
    assert.equal(signedOut.status, SIGNED_OUT_STATUS, signedOut.text);
    assert.equal(signedOut.text, '', 'the 204 carried a body');

    const cleared = sessionSetCookie(signedOut);
    assert.ok(cleared, 'sign-out set no clearing cookie');
    assert.match(cleared, CLEARED_COOKIE_PATTERN);
    // The clearing cookie must carry the attributes of the one it replaces, or the browser keeps
    // the original and the person stays signed in on screen.
    assert.match(cleared, /HttpOnly/i);
    assert.match(cleared, /Path=\//i);
    assert.match(cleared, /SameSite=Lax/i);
  });

  test('leaves the revoked session authorizing nothing', async () => {
    const { cookie, csrfToken } = await registerAccount('revoked');
    await call(SIGN_OUT_PATH, { method: 'POST', cookie, csrfToken });
    assert.equal((await call(CURRENT_USER_PATH, { cookie })).status, 401);
  });

  test('keeps an independent session of the same user working', async () => {
    const { email, cookie, csrfToken } = await registerAccount('other-device');
    const otherDevice = await signInFromSecondDevice(email);
    assert.notEqual(otherDevice.cookie, cookie, 'the second sign-in reused the first session');

    await call(SIGN_OUT_PATH, { method: 'POST', cookie, csrfToken });

    const survivor = await call(CURRENT_USER_PATH, { cookie: otherDevice.cookie });
    assert.equal(survivor.status, 200, survivor.text);
    assert.equal(survivor.json.user.email, email);
  });
});

describe('signing out is always safe to repeat', () => {
  test('answers 204 when the session was already revoked', async () => {
    const { cookie, csrfToken } = await registerAccount('repeat');
    await call(SIGN_OUT_PATH, { method: 'POST', cookie, csrfToken });
    const repeated = await call(SIGN_OUT_PATH, { method: 'POST', cookie, csrfToken });
    assert.equal(repeated.status, SIGNED_OUT_STATUS, repeated.text);
    assert.ok(sessionSetCookie(repeated), 'the repeat cleared no cookie');
  });

  test('answers 204 when the request carries no session at all', async () => {
    const withoutSession = await call(SIGN_OUT_PATH, { method: 'POST' });
    assert.equal(withoutSession.status, SIGNED_OUT_STATUS, withoutSession.text);
    assert.ok(sessionSetCookie(withoutSession), 'the request cleared no cookie');
  });

  test('answers 204 when the token names no session', async () => {
    const unknown = await call(SIGN_OUT_PATH, { method: 'POST', cookie: UNKNOWN_SESSION_COOKIE });
    assert.equal(unknown.status, SIGNED_OUT_STATUS, unknown.text);
  });
});

describe('the origin is checked before the session is', () => {
  test('refuses a foreign origin even when there is no session to end', async () => {
    const refused = await call(SIGN_OUT_PATH, { method: 'POST', origin: foreignOrigin });
    assert.equal(refused.status, REFUSED_STATUS, refused.text);
    assert.equal(refused.json.code, ORIGIN_NOT_ALLOWED_CODE);
    assert.deepEqual(refused.setCookie, [], 'the refusal touched the cookie');
  });

  test('does not let a foreign page sign a live session out', async () => {
    const { cookie, csrfToken } = await registerAccount('foreign-logout');
    const refused = await call(SIGN_OUT_PATH, {
      method: 'POST',
      cookie,
      csrfToken,
      origin: foreignOrigin,
    });
    assert.equal(refused.status, REFUSED_STATUS, refused.text);
    assert.equal((await call(CURRENT_USER_PATH, { cookie })).status, 200, 'a foreign page revoked the session');
  });
});
