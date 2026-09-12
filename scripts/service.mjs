// Where the assembled stack is and what it is made of. The smoke check and the acceptance suites
// reach the same Compose project, so these coordinates are declared once here rather than in each
// script that happens to need them, and a changed port or database name is one edit.
import { execFileSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

/**
 * The address the Compose project publishes the application on, and the origin its own local
 * profile allows. `APP_PORT` in `.env` decides the published port, so a stack started on another
 * one is reached by stating it here.
 */
export const SERVICE_ORIGIN = process.env.ACCEPTANCE_BASE ?? 'http://127.0.0.1:8080';

/**
 * The session cookie the service issues. It is the API contract's session security scheme, which
 * the backend declares as `sessions.CookieName`; nothing here reads the cookie to decide anything,
 * so the suites observe the line the service actually set rather than a value they chose.
 */
export const SESSION_COOKIE_NAME = 'carsharing_session';

/** The database the stack runs on, reached through the migrator role that owns its schema. */
const POSTGRES_SERVICE = 'postgres';
const POSTGRES_ROLE = 'carsharing_migrator';
const POSTGRES_DATABASE = 'carsharing';

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
