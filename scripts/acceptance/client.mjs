// Shared request helper for the acceptance suites. They speak to the assembled stack over real
// HTTP, so cookie attributes, status codes and transaction outcomes are observed the way the
// running service presents them rather than asserted about the code that produces them.
import { execFileSync } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');

export const base = process.env.ACCEPTANCE_BASE ?? 'http://127.0.0.1:8080';

/** The origin the local profile allows. A suite uses it unless it is testing a refusal. */
export const allowedOrigin = base;

export const password = 'correcthorsebattery';

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
  const response = await fetch(base + path, {
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

/** The cookie pair a browser would send back, or null when the response set no session cookie. */
export function sessionCookie(response) {
  const issued = sessionSetCookie(response);
  return issued ? issued.split(';')[0] : null;
}

/** The whole Set-Cookie line for the session, so a suite can assert its attributes. */
export function sessionSetCookie(response) {
  return response.setCookie.find((value) => value.startsWith('carsharing_session=')) ?? null;
}

/** Registers a new account and returns the session it established. */
export async function registerAccount(prefix, overrides = {}) {
  const email = overrides.email ?? newEmail(prefix);
  const response = await call('/api/v1/auth/register', {
    method: 'POST',
    body: { email, password: overrides.password ?? password },
  });
  return { email, response, cookie: sessionCookie(response), csrfToken: response.json?.csrf_token };
}

export function compose(...args) {
  return execFileSync('docker', ['compose', ...args], {
    cwd: root,
    encoding: 'utf8',
    timeout: 120_000,
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim();
}

export function sql(query) {
  return compose(
    'exec', '-T', 'postgres',
    'psql', '-U', 'carsharing_migrator', '-d', 'carsharing',
    '-At', '-v', 'ON_ERROR_STOP=1', '-c', query,
  );
}

/** Waits for the API to answer, so a suite started beside a restart does not race it. */
export async function waitForReady() {
  for (let attempt = 0; attempt < 60; attempt += 1) {
    try {
      const response = await fetch(`${base}/api/v1/health/ready`, { signal: AbortSignal.timeout(3000) });
      if (response.ok) return;
    } catch {
      // Keep waiting within the bounded deadline.
    }
    await delay(1000);
  }
  throw new Error('API did not become ready');
}
