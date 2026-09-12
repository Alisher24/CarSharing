// Which page may act, and with what token. Observed on the real HTTP boundary, because the point
// of both rules is what the running service refuses before anything is created.
import assert from 'node:assert/strict';
import { before, beforeEach, describe, test } from 'node:test';
import {
  call,
  CURRENT_USER_PATH,
  foreignOrigin,
  newEmail,
  password,
  registerAccount,
  REGISTRATION_PATH,
  registrationRequest,
  resetRateLimits,
  sessionCookie,
  SIGN_IN_PATH,
  signInRequest,
  SIGN_OUT_PATH,
  waitForReady,
} from './client.mjs';

const REFUSED_STATUS = 403;
const CREATED_STATUS = 201;
const ACCEPTED_STATUS = 200;
const SIGNED_OUT_STATUS = 204;
const ORIGIN_NOT_ALLOWED_CODE = 'ORIGIN_NOT_ALLOWED';
const CSRF_INVALID_CODE = 'CSRF_INVALID';

const MINIMUM_CSRF_TOKEN_LENGTH = 20;
const WRONG_CSRF_TOKEN = 'not-the-token-this-session-was-issued';

// Every mutation the origin rule covers, with the request that performs it. Only the registration
// creates something a refusal could have left behind, so only it carries a side-effect check. The
// request is built per attempt so each one carries a fresh address.
const MUTATIONS = [
  { name: 'registration', path: REGISTRATION_PATH, buildRequest: registrationRequest, checkNoSideEffect },
  { name: 'sign-in', path: SIGN_IN_PATH, buildRequest: (email) => signInRequest(email) },
  { name: 'sign-out', path: SIGN_OUT_PATH, buildRequest: () => ({ method: 'POST' }) },
];

before(waitForReady);

// Every suite shares one address, so each test starts with the rate limits untouched by the last.
beforeEach(resetRateLimits);

/** A refused registration must leave the address free, so the refusal created no account. */
async function checkNoSideEffect(email) {
  const created = await call(REGISTRATION_PATH, registrationRequest(email));
  assert.equal(created.status, CREATED_STATUS, 'the refused request created an account');
}

describe('a mutation from an origin this application does not allow', () => {
  for (const mutation of MUTATIONS) {
    test(`refuses ${mutation.name} from a foreign origin without a side effect`, async () => {
      const email = newEmail('foreign');
      const refused = await call(mutation.path, { ...mutation.buildRequest(email), origin: foreignOrigin });
      assert.equal(refused.status, REFUSED_STATUS, refused.text);
      assert.equal(refused.json.code, ORIGIN_NOT_ALLOWED_CODE);
      assert.deepEqual(refused.setCookie, [], 'a refused request touched the cookie');
      if (mutation.checkNoSideEffect) await mutation.checkNoSideEffect(email);
    });

    test(`refuses ${mutation.name} with no Origin at all`, async () => {
      const email = newEmail('absent-origin');
      const refused = await call(mutation.path, { ...mutation.buildRequest(email), omitOrigin: true });
      assert.equal(refused.status, REFUSED_STATUS, refused.text);
      assert.equal(refused.json.code, ORIGIN_NOT_ALLOWED_CODE);
      assert.deepEqual(refused.setCookie, []);
    });
  }

  test('lets an allowed origin through', async () => {
    const { response } = await registerAccount('allowed');
    assert.equal(response.status, CREATED_STATUS, response.text);
  });
});

describe('the CSRF token is bound to one session', () => {
  test('is issued inside the snapshot', async () => {
    const { response } = await registerAccount('csrf-issued');
    assert.equal(typeof response.json.csrf_token, 'string');
    assert.ok(response.json.csrf_token.length > MINIMUM_CSRF_TOKEN_LENGTH, 'the CSRF token is too short');
  });

  test('refuses an authenticated mutation that carries none', async () => {
    const { cookie } = await registerAccount('csrf-absent');
    const refused = await call(SIGN_OUT_PATH, { method: 'POST', cookie });
    assert.equal(refused.status, REFUSED_STATUS, refused.text);
    assert.equal(refused.json.code, CSRF_INVALID_CODE);
    const restored = await call(CURRENT_USER_PATH, { cookie });
    assert.equal(restored.status, ACCEPTED_STATUS, 'the refusal revoked the session');
  });

  test('refuses an authenticated mutation that carries the wrong one', async () => {
    const { cookie } = await registerAccount('csrf-wrong');
    const refused = await call(SIGN_OUT_PATH, { method: 'POST', cookie, csrfToken: WRONG_CSRF_TOKEN });
    assert.equal(refused.status, REFUSED_STATUS, refused.text);
    assert.equal(refused.json.code, CSRF_INVALID_CODE);
    const restored = await call(CURRENT_USER_PATH, { cookie });
    assert.equal(restored.status, ACCEPTED_STATUS, 'the refusal revoked the session');
  });

  test('accepts the token its own session was issued', async () => {
    const { cookie, csrfToken } = await registerAccount('csrf-right');
    const accepted = await call(SIGN_OUT_PATH, { method: 'POST', cookie, csrfToken });
    assert.equal(accepted.status, SIGNED_OUT_STATUS, accepted.text);
  });
});

describe('replacing a session retires the token it issued', () => {
  test("a new session never accepts the previous session's CSRF token", async () => {
    const { email, cookie, csrfToken } = await registerAccount('csrf-replaced');
    const replaced = await call(SIGN_IN_PATH, {
      method: 'POST',
      body: { email, password },
      cookie,
    });
    assert.equal(replaced.status, ACCEPTED_STATUS, replaced.text);
    const replacementCookie = sessionCookie(replaced);
    assert.notEqual(replaced.json.csrf_token, csrfToken, 'the replacement reused the old CSRF token');

    const stale = await call(SIGN_OUT_PATH, { method: 'POST', cookie: replacementCookie, csrfToken });
    assert.equal(stale.status, REFUSED_STATUS, stale.text);
    assert.equal(stale.json.code, CSRF_INVALID_CODE);

    const original = await call(CURRENT_USER_PATH, { cookie });
    assert.equal(original.status, 401, 'the replaced session still authorizes');
  });
});
