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
 * on a mutation; a suite testing the origin rule overrides or omits it.
 */
export async function call(path, options = {}) {
  const { method = 'GET', body, cookie, csrfToken, origin = allowedOrigin, omitOrigin = false } = options;
  const headers = {};
  if (!omitOrigin) headers.Origin = origin;
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (cookie) headers.Cookie = cookie;
  if (csrfToken !== undefined) headers['X-CSRF-Token'] = csrfToken;
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
 */
export async function signInFromSecondDevice(email) {
  const response = await call(SIGN_IN_PATH, signInRequest(email, true));
  assert.equal(response.status, 200, response.text);
  return { response, cookie: sessionCookie(response), csrfToken: response.json.csrf_token };
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
 * Clears every rate-limit counter. The suites share one address, so without this the limits would
 * refuse the accounts a later suite needs. Clearing a counter is the harness standing in for the
 * passage of time, which is also how access returns in production.
 */
export function resetRateLimits() {
  sql('DELETE FROM rate_limit_counters');
}
