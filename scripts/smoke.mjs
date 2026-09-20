import assert from 'node:assert/strict';
import { setTimeout as delay } from 'node:timers/promises';
import { POSTGRES_DATABASE, POSTGRES_SERVICE, SERVICE_ORIGIN, composeWith, sql } from './service.mjs';

const LOCAL_ORIGIN_PATTERN = /^http:\/\/(127\.0\.0\.1|localhost):\d+$/;
const ORIGIN_ARGUMENT_INDEX = 2;

const READINESS_ATTEMPTS = 60;
const READINESS_RETRY_DELAY_MS = 1_000;
const READINESS_REQUEST_TIMEOUT_MS = 3_000;
const OUTAGE_REQUEST_TIMEOUT_MS = 5_000;

const MIGRATION_SERVICE = 'migrate';
const MIGRATION_UP_ARGUMENT = 'up';
const SEED_SERVICE = 'seed';
const DEMO_COMPOSE_PROFILE = 'demo';

const API_PREFIX = '/api/v1';
const READINESS_PATH = `${API_PREFIX}/health/ready`;
const LIVENESS_PATH = `${API_PREFIX}/health/live`;
const BOOTSTRAP_SEED = 'bootstrap-v1';

const EXPECTED_CURRENCY = 'KGS';
const EXPECTED_CITY = 'Бишкек';
const EXPECTED_TIMEZONE = 'Asia/Bishkek';
const SERVER_TIME_PATTERN = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;
const POSTGIS_VERSION_PATTERN = /^3\.6/;
const ROOT_ELEMENT_PATTERN = /<div id="root"><\/div>/;
const SELF_ONLY_CONTENT_POLICY_PATTERN = /default-src 'self'/;

const RESOURCE_NOT_FOUND_CODE = 'RESOURCE_NOT_FOUND';
const AUTHENTICATION_REQUIRED_CODE = 'AUTHENTICATION_REQUIRED';
const SERVICE_UNAVAILABLE_CODE = 'SERVICE_UNAVAILABLE';

const POSTGIS_VERSION_QUERY = 'SELECT postgis_version()';
const BOOTSTRAP_METADATA_CREATED_AT_QUERY = 'SELECT created_at FROM bootstrap_metadata';
const BOOTSTRAP_SEED_APPLIED_AT_QUERY = `SELECT applied_at FROM seed_runs WHERE name = '${BOOTSTRAP_SEED}'`;
const BOOTSTRAP_SEED_COUNT_QUERY = `SELECT count(*) FROM seed_runs WHERE name = '${BOOTSTRAP_SEED}'`;

// Retired health URLs, planned operations and the internal API must all be unreachable from outside.
const UNREACHABLE_PATHS = ['/health/live', '/health/ready', '/api/health', '/internal/v1/simulation/tick'];

// The public catalog is served through the same proxy, with no account and no cookie.
const PUBLIC_CATALOG_PATHS = ['/api/v1/vehicles', '/api/v1/zones', '/api/v1/tariffs'];

/**
 * The role the API runs as, read from the database rather than restated here. The service records
 * it when it connects, so this is the role actually in use and not the one the deployment intended.
 * The migrator's own sessions are the ones psql names, which is how they are filtered out; pgx
 * connects without an application name of its own.
 */
function applicationRole() {
  const roles = sql(`SELECT DISTINCT usename FROM pg_stat_activity
     WHERE datname = '${POSTGRES_DATABASE}' AND coalesce(application_name, '') <> 'psql'`);
  return roles.split('\n').find((role) => role !== '') ?? '';
}

/** Runs a one-off compose service, as the migration and seed containers are meant to be run. */
function runOneOffService(service, serviceArguments, composeOptions = []) {
  return composeWith({}, ...composeOptions, 'run', '--rm', service, ...serviceArguments);
}

/** The seed profile is demo data, so a run of it never reaches an installation without that profile. */
function runDemoSeed(composeOptions = []) {
  return runOneOffService(SEED_SERVICE, [], ['--profile', DEMO_COMPOSE_PROFILE, ...composeOptions]);
}

function assertReadinessPayload(payload) {
  assert.equal(payload.status, 'ok');
  assert.equal(payload.currency, EXPECTED_CURRENCY);
  assert.equal(payload.city, EXPECTED_CITY);
  assert.equal(payload.timezone, EXPECTED_TIMEZONE);
  assert.match(payload.server_time, SERVER_TIME_PATTERN);
}

/**
 * Waits until the API reports itself ready, and returns the payload it answered with. Every attempt
 * is abandoned after READINESS_REQUEST_TIMEOUT_MS, so a service that accepts connections without
 * answering cannot hold the wait open past its deadline.
 */
async function awaitReady(origin) {
  for (let attempt = 0; attempt < READINESS_ATTEMPTS; attempt += 1) {
    try {
      const response = await fetch(`${origin}${READINESS_PATH}`, {
        signal: AbortSignal.timeout(READINESS_REQUEST_TIMEOUT_MS),
      });
      if (response.ok) {
        const payload = await response.json();
        assertReadinessPayload(payload);
        assert.equal(response.headers.get('cache-control'), 'no-store');
        assert.ok(response.headers.get('x-request-id'));
        return payload;
      }
    } catch {
      // Keep waiting within the bounded deadline: the stack is still starting or reconnecting.
    }
    await delay(READINESS_RETRY_DELAY_MS);
  }
  throw new Error('API did not become ready');
}

async function checkFrontendIsServed(origin) {
  const response = await fetch(origin);
  assert.equal(response.status, 200);
  assert.match(await response.text(), ROOT_ELEMENT_PATTERN);
  assert.match(response.headers.get('content-security-policy') ?? '', SELF_ONLY_CONTENT_POLICY_PATTERN);
}

async function checkUnreachablePaths(origin) {
  for (const path of UNREACHABLE_PATHS) {
    const response = await fetch(`${origin}${path}`);
    assert.equal(response.status, 404, path);
    const error = await response.json();
    assert.equal(error.code, RESOURCE_NOT_FOUND_CODE);
    assert.equal(error.request_id, response.headers.get('x-request-id'));
    assert.equal(response.headers.get('cache-control'), 'no-store');
  }
}

async function checkPublicCatalogIsServed(origin) {
  for (const path of PUBLIC_CATALOG_PATHS) {
    const response = await fetch(`${origin}${path}`);
    assert.equal(response.status, 200, path);
    assert.ok(Array.isArray((await response.json()).items), path);
  }
}

// An implemented operation behind a session answers as unauthenticated rather than as absent, so a
// caller can tell "sign in" from "no such resource".
async function checkSessionRequirementIsDistinguishable(origin) {
  const response = await fetch(`${origin}${API_PREFIX}/me`);
  assert.equal(response.status, 401);
  assert.equal((await response.json()).code, AUTHENTICATION_REQUIRED_CODE);
}

function checkDatabaseRoleIsUnprivileged() {
  const role = applicationRole();
  assert.ok(role, 'the API recorded no application role, so its privileges cannot be checked');
  assert.equal(sql(`SELECT (rolsuper OR rolcreatedb OR rolcreaterole) FROM pg_roles WHERE rolname = '${role}'`), 'f');
  assert.equal(sql(`SELECT has_table_privilege('${role}', 'bootstrap_metadata', 'INSERT')`), 'f');
  assert.match(sql(POSTGIS_VERSION_QUERY), POSTGIS_VERSION_PATTERN);
  return role;
}

/** Migrations and seeds must be repeatable, so a second run changes neither row nor timestamp. */
function checkRepeatedMigrationAndSeedAreIdempotent() {
  const metadataCreatedAt = sql(BOOTSTRAP_METADATA_CREATED_AT_QUERY);
  runOneOffService(MIGRATION_SERVICE, [MIGRATION_UP_ARGUMENT]);
  assert.equal(sql(BOOTSTRAP_METADATA_CREATED_AT_QUERY), metadataCreatedAt);

  runDemoSeed();
  const seedAppliedAt = sql(BOOTSTRAP_SEED_APPLIED_AT_QUERY);
  runDemoSeed();
  assert.equal(sql(BOOTSTRAP_SEED_COUNT_QUERY), '1');
  assert.equal(sql(BOOTSTRAP_SEED_APPLIED_AT_QUERY), seedAppliedAt);

  return { metadataCreatedAt, seedAppliedAt };
}

/** A restart of the API must not lose the migration and seed state the previous checks recorded. */
function checkRecordedStatePersists(recordedState) {
  assert.equal(sql(BOOTSTRAP_METADATA_CREATED_AT_QUERY), recordedState.metadataCreatedAt);
  assert.equal(sql(BOOTSTRAP_SEED_APPLIED_AT_QUERY), recordedState.seedAppliedAt);
}

// Exercises dependency failure while the API process stays alive, then restores it even on failure.
async function checkOutageIsReportedAndRecovered(origin) {
  composeWith({}, 'stop', POSTGRES_SERVICE);
  try {
    const response = await fetch(`${origin}${READINESS_PATH}`, {
      signal: AbortSignal.timeout(OUTAGE_REQUEST_TIMEOUT_MS),
    });
    assert.equal(response.status, 503);
    const error = await response.json();
    assert.equal(error.code, SERVICE_UNAVAILABLE_CODE);
    assert.equal(error.request_id, response.headers.get('x-request-id'));
    assert.equal((await fetch(`${origin}${LIVENESS_PATH}`)).status, 200);
  } finally {
    composeWith({}, 'start', POSTGRES_SERVICE);
  }
  await awaitReady(origin);
}

async function main() {
  const origin = process.argv[ORIGIN_ARGUMENT_INDEX] ?? SERVICE_ORIGIN;
  if (!LOCAL_ORIGIN_PATTERN.test(origin)) {
    throw new Error(`Smoke test accepts only a local HTTP origin, for example ${origin}`);
  }

  await awaitReady(origin);
  await checkFrontendIsServed(origin);
  await checkUnreachablePaths(origin);
  await checkPublicCatalogIsServed(origin);
  await checkSessionRequirementIsDistinguishable(origin);
  checkDatabaseRoleIsUnprivileged();
  const recordedState = checkRepeatedMigrationAndSeedAreIdempotent();
  await checkOutageIsReportedAndRecovered(origin);
  checkRecordedStatePersists(recordedState);
  console.log(
    'PASS: frontend/proxy, live API/PostGIS, public catalog, runtime role,',
    'repeated migrations/seed, outage recovery and persistent data.',
  );
}

await main();
