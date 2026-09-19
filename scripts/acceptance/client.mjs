// Shared request helper for the acceptance suites. They speak to the assembled stack over real
// HTTP, so cookie attributes, status codes and transaction outcomes are observed the way the
// running service presents them rather than asserted about the code that produces them.
import assert from 'node:assert/strict';
import { setTimeout as delay } from 'node:timers/promises';
import { SERVICE_ORIGIN, SESSION_COOKIE_NAME, compose, composeWith, sql } from '../service.mjs';

const READINESS_ATTEMPTS = 60;
const READINESS_RETRY_DELAY_MS = 1_000;
const READINESS_REQUEST_TIMEOUT_MS = 3_000;

const HEALTH_PREFIX = '/api/v1/health';
const READINESS_PATH = `${HEALTH_PREFIX}/ready`;
const AUTH_PREFIX = '/api/v1/auth';
export const REGISTRATION_PATH = `${AUTH_PREFIX}/register`;
export const SIGN_IN_PATH = `${AUTH_PREFIX}/login`;
export const SIGN_OUT_PATH = `${AUTH_PREFIX}/logout`;
export const CURRENT_USER_PATH = '/api/v1/me';

const SESSION_COOKIE_PREFIX = `${SESSION_COOKIE_NAME}=`;

/** The code the service answers with when it could not decide an attempt at all. */
const SERVICE_UNAVAILABLE_CODE = 'SERVICE_UNAVAILABLE';

/** How many times a check asks again for a decision the service was too busy to make. */
const BUSY_ATTEMPTS = 10;
const BUSY_RETRY_DELAY_MS = 200;

/** How long a suite that follows a burst waits for the hasher to have room again. */
const SETTLE_ATTEMPTS = 60;
const SETTLE_RETRY_DELAY_MS = 500;

export const serviceOrigin = SERVICE_ORIGIN;

export { compose, composeWith, sql };

/** The origin the local profile allows. A suite uses it unless it is testing a refusal. */
export const allowedOrigin = serviceOrigin;

export const password = 'correcthorsebattery';

/** A password no account holds, for the suites that need a rejected sign-in attempt. */
export const wrongPassword = 'wrongpasswordvalue';

/** An origin the local profile refuses, for the suites that observe the refusal. */
export const foreignOrigin = 'http://attacker.example';

/** A fresh canonical address, so a suite never depends on the state another suite left behind. */
export function newEmail(prefix) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@example.test`;
}

/**
 * call makes one request and returns everything a suite asserts about: the status, the parsed
 * body and the Set-Cookie headers. Origin is attached by default because the contract requires it
 * on a mutation; a suite testing the origin rule overrides or omits it. Headers a contract declares
 * for one operation — a command key, above all — are passed in by the suite that sends them.
 */
export async function call(path, options = {}) {
  const {
    method = 'GET',
    body,
    cookie,
    csrfToken,
    origin = allowedOrigin,
    omitOrigin = false,
    headers: declared = {},
  } = options;
  const headers = {};
  if (!omitOrigin) headers.Origin = origin;
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (cookie) headers.Cookie = cookie;
  if (csrfToken !== undefined) headers['X-CSRF-Token'] = csrfToken;
  Object.assign(headers, declared);
  const response = await fetch(serviceOrigin + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await response.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = null;
  }
  return {
    status: response.status,
    headers: response.headers,
    text,
    json,
    setCookie: response.headers.getSetCookie(),
  };
}

/** The registration request a fresh address needs, for the suites that register directly. */
export function registrationRequest(email, registrationPassword = password) {
  return { method: 'POST', body: { email, password: registrationPassword } };
}

/**
 * The sign-in request for one address. A test that observes a limit sends the wrong password and
 * spends the budget; one that proves a correct sign-in costs nothing asks for the right password.
 */
export function signInRequest(email, withCorrectPassword = false) {
  return {
    method: 'POST',
    body: { email, password: withCorrectPassword ? password : wrongPassword },
  };
}

/** The cookie pair a browser would send back, or null when the response set no session cookie. */
export function sessionCookie(response) {
  const issued = sessionSetCookie(response);
  return issued ? issued.split(';')[0] : null;
}

/** The whole Set-Cookie line for the session, so a suite can assert its attributes. */
export function sessionSetCookie(response) {
  return response.setCookie.find((value) => value.startsWith(SESSION_COOKIE_PREFIX)) ?? null;
}

/** Registers a new account and returns the session it established. */
export async function registerAccount(prefix, overrides = {}) {
  const email = overrides.email ?? newEmail(prefix);
  const response = await call(REGISTRATION_PATH, registrationRequest(email, overrides.password));
  return { email, response, cookie: sessionCookie(response), csrfToken: response.json?.csrf_token };
}

/**
 * Signs one address in again without presenting the first session, as a second device would. The
 * replacement retires tokens the first session was issued, so the caller compares the two.
 *
 * A suite that has just sent a burst of sign-ins may find every hashing slot busy, which the service
 * answers as a service failure rather than as a wrong password. That is the service saying it could
 * not decide the attempt, so the caller asks again rather than reading it as an answer.
 */
export async function signInFromSecondDevice(email) {
  for (let attempt = 0; ; attempt += 1) {
    const response = await call(SIGN_IN_PATH, signInRequest(email, true));
    const busy = response.json?.code === SERVICE_UNAVAILABLE_CODE;
    if (!busy || attempt >= BUSY_ATTEMPTS) {
      assert.equal(response.status, 200, response.text);
      return { response, cookie: sessionCookie(response), csrfToken: response.json.csrf_token };
    }
    await delay(BUSY_RETRY_DELAY_MS);
  }
}

/** Waits for the API to answer, so a suite started beside a restart does not race it. */
export async function waitForReady() {
  for (let attempt = 0; attempt < READINESS_ATTEMPTS; attempt += 1) {
    try {
      const response = await fetch(`${serviceOrigin}${READINESS_PATH}`, {
        signal: AbortSignal.timeout(READINESS_REQUEST_TIMEOUT_MS),
      });
      if (response.ok) return;
    } catch {
      // Keep waiting within the bounded deadline: the stack is still starting or reconnecting.
    }
    await delay(READINESS_RETRY_DELAY_MS);
  }
  throw new Error('API did not become ready');
}

/**
 * Waits for the hasher to have room again and clears the budgets a burst spent.
 *
 * One API process admits a fixed number of hashes at a time and refuses the rest rather than queueing
 * them, which is what a suite that asks for a burst on purpose observes. The burst is a fact of this
 * shared stack, so a suite that follows one waits here: an attempt answered `503` is the service
 * saying it could not decide rather than an answer about credentials, and a budget a burst spent is
 * not something the next suite should inherit. The wait is bounded, so a stack that never recovers
 * fails the suite that called this rather than hanging it.
 */
export async function settleAfterBurst() {
  const email = newEmail('settle');
  const registration = await call(REGISTRATION_PATH, registrationRequest(email));
  resetRateLimits();
  if (registration.status !== 201) return;

  for (let attempt = 0; attempt < SETTLE_ATTEMPTS; attempt += 1) {
    const response = await call(SIGN_IN_PATH, signInRequest(email, true));
    // A granted attempt is answered with the session it issued; a refused one with invalid
    // credentials, because the address this probe signs is one no account holds. Both mean the
    // attempt was decided, which is what this wait is about.
    if (response.json?.code !== SERVICE_UNAVAILABLE_CODE) {
      resetRateLimits();
      return;
    }
    await delay(SETTLE_RETRY_DELAY_MS);
  }
  throw new Error('the hasher never had room again after a burst');
}

/**
 * Polls a request until the service refuses it, which is how a suite observes a limit it has just
 * exhausted. Every attempt before that must answer as the expected status, so a refusal is what
 * ends the wait rather than an unrelated failure.
 */
export async function callUntilRefused(action, { allowedAttempts, expectedStatus, refusalStatus, limitName }) {
  for (let attempt = 0; attempt < allowedAttempts + 1; attempt += 1) {
    const response = await action();
    if (response.status === refusalStatus) return response;
    assert.equal(response.status, expectedStatus, response.text);
  }
  throw new Error(`${limitName} never refused an attempt`);
}

/**
 * Clears every rate-limit counter, which is the harness standing in for the passage of time: in
 * production a window ends on its own, and here a suite ends it between checks.
 */
export function resetRateLimits() {
  sql('DELETE FROM rate_limit_counters');
}
