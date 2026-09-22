// Where the assembled stack is and what it is made of. The smoke check and the acceptance suites
// reach the same Compose project, so these coordinates are declared once here rather than in each
// script that happens to need them, and a changed port or database name is one edit.
import { execFileSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';

export const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

/**
 * The address the Compose project publishes the application on, and the origin its own local
 * profile allows. `APP_PORT` in `.env` decides the published port, so a stack started on another
 * one is reached by stating it here.
 */
export const SERVICE_ORIGIN = process.env.ACCEPTANCE_BASE ?? 'http://127.0.0.1:8080';

/**
 * The path the API answers its readiness probe on, which is where a caller waits for the stack.
 * `httpapi.ReadyPath` declares the same path for the process that serves it.
 */
export const READINESS_PATH = '/api/v1/health/ready';

/**
 * How long a caller waits for that answer: the attempts, the delay between them, and the bound on one
 * attempt, so a service that accepts a connection without answering cannot hold the wait open past
 * its deadline. The smoke check and the suites wait the same way rather than each stating a budget.
 */
const READINESS_ATTEMPTS = 60;
const READINESS_RETRY_DELAY_MS = 1_000;
const READINESS_REQUEST_TIMEOUT_MS = 3_000;

/**
 * Waits until the API reports itself ready and returns the answer it gave, so a caller can hold the
 * payload and the headers of that answer to what the readiness operation promises.
 */
export async function waitForReady(origin = SERVICE_ORIGIN) {
  for (let attempt = 0; attempt < READINESS_ATTEMPTS; attempt += 1) {
    try {
      const response = await fetch(`${origin}${READINESS_PATH}`, {
        signal: AbortSignal.timeout(READINESS_REQUEST_TIMEOUT_MS),
      });
      if (response.ok) return response;
    } catch {
      // Keep waiting within the bounded deadline: the stack is still starting or reconnecting.
    }
    await delay(READINESS_RETRY_DELAY_MS);
  }
  throw new Error('API did not become ready');
}

/**
 * The address the mail stub publishes its inbox on, which is the loopback address of the host and the
 * port the stub's configuration documents. It is the one surface of that process a check reaches from
 * the host: the internal listener is not published at all, so a check that needs it sends its request
 * from a container on the same network.
 */
export const MAILBOX_ORIGIN = process.env.ACCEPTANCE_MAILBOX ?? 'http://127.0.0.1:8025';

/**
 * The session cookie the service issues. It is the API contract's session security scheme, which
 * the backend declares as `sessions.CookieName`; nothing here reads the cookie to decide anything,
 * so the suites observe the line the service actually set rather than a value they chose.
 */
export const SESSION_COOKIE_NAME = 'carsharing_session';

/** The database the stack runs on, reached through the migrator role that owns its schema. */
export const POSTGRES_SERVICE = 'postgres';
export const POSTGRES_ROLE = 'carsharing_migrator';
export const POSTGRES_DATABASE = 'carsharing';

const COMPOSE_TIMEOUT_MS = 180_000;

const DATABASE_QUERY_ARGUMENTS = [
  'exec',
  '-T',
  POSTGRES_SERVICE,
  'psql',
  '-U',
  POSTGRES_ROLE,
  '-d',
  POSTGRES_DATABASE,
  '-At',
  '-v',
  'ON_ERROR_STOP=1',
  '-c',
];

export function compose(...args) {
  return composeWith({}, ...args);
}

/**
 * Runs a compose command with extra environment. Compose substitutes these into the service
 * definition, so recreating a service this way is how a suite proves the running service reads its
 * configuration rather than a constant compiled into it.
 */
export function composeWith(environment, ...args) {
  return execFileSync('docker', ['compose', ...args], {
    cwd: repositoryRoot,
    encoding: 'utf8',
    timeout: COMPOSE_TIMEOUT_MS,
    stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, ...environment },
  }).trim();
}

/**
 * The psql invocation every check shares: the migrator owns the database the assembled stack runs
 * on, and a query that fails must stop the compose command rather than return partial output.
 */
export function sql(query) {
  return compose(...DATABASE_QUERY_ARGUMENTS, query);
}

/**
 * Runs one statement as a role of the database and reports whether it was accepted.
 *
 * The privilege under check belongs to the role rather than to the schema owner, so a statement the
 * migrator runs proves nothing about it: this sets the role inside one transaction, which is rolled
 * back, and answers whether the database refused what was written. The statement never names a row
 * and never commits, so a template that changes nothing is safe to present: what is observed is the
 * privilege, not the effect.
 */
export function asRole(role, statement) {
  try {
    sql(`BEGIN; SET LOCAL ROLE ${role}; ${statement}; ROLLBACK;`);
    return { accepted: true, refusal: '' };
  } catch (failure) {
    return { accepted: false, refusal: String(failure.stderr ?? failure.message) };
  }
}
