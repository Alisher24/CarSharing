import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const base = process.argv[2] ?? 'http://127.0.0.1:8080';
if (!/^http:\/\/(127\.0\.0\.1|localhost):\d+$/.test(base)) {
  throw new Error('Smoke test accepts only a local HTTP origin, for example http://127.0.0.1:8080');
}
function compose(...args) {
  return execFileSync('docker', ['compose', ...args], {
    cwd: root, encoding: 'utf8', timeout: 120_000, stdio: ['ignore', 'pipe', 'pipe'],
  }).trim();
}
function sql(query) {
  return compose(
    'exec', '-T', 'postgres',
    'psql', '-U', 'carsharing_migrator', '-d', 'carsharing',
    '-At', '-v', 'ON_ERROR_STOP=1', '-c', query,
  );
}
async function ready() {
  for (let i = 0; i < 60; i++) {
    try {
      const response = await fetch(`${base}/api/v1/health/ready`, { signal: AbortSignal.timeout(3000) });
      if (response.ok) {
        const data = await response.json();
        assert.equal(data.status, 'ok');
        assert.equal(data.currency, 'KGS');
        assert.equal(data.city, 'Бишкек');
        assert.equal(data.timezone, 'Asia/Bishkek');
        assert.match(data.server_time, /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/);
        assert.equal(response.headers.get('cache-control'), 'no-store');
        assert.ok(response.headers.get('x-request-id'));
        return data;
      }
    } catch { /* Wait for startup/reconnection within the bounded deadline. */ }
    await delay(1000);
  }
  throw new Error('API did not become ready');
}

await ready();
const page = await fetch(base);
assert.equal(page.status, 200);
assert.match(await page.text(), /<div id="root"><\/div>/);
assert.match(page.headers.get('content-security-policy') ?? '', /default-src 'self'/);
// Retired health URLs, planned operations and the internal API must all be unreachable from outside.
const unreachable = [
  '/health/live',
  '/health/ready',
  '/api/health',
  '/api/v1/vehicles',
  '/internal/v1/simulation/tick',
];
for (const path of unreachable) {
  const response = await fetch(`${base}${path}`);
  assert.equal(response.status, 404, path);
  const error = await response.json();
  assert.equal(error.code, 'RESOURCE_NOT_FOUND');
  assert.equal(error.request_id, response.headers.get('x-request-id'));
  assert.equal(response.headers.get('cache-control'), 'no-store');
}
// An implemented operation behind a session answers as unauthenticated rather than as absent, so a
// caller can tell "sign in" from "no such resource".
const unauthenticated = await fetch(`${base}/api/v1/me`);
assert.equal(unauthenticated.status, 401);
assert.equal((await unauthenticated.json()).code, 'AUTHENTICATION_REQUIRED');

const appRolePrivileged = 'SELECT rolsuper OR rolcreatedb OR rolcreaterole'
  + " FROM pg_roles WHERE rolname = 'carsharing_app'";
assert.equal(sql(appRolePrivileged), 'f');
assert.equal(sql("SELECT has_table_privilege('carsharing_app', 'bootstrap_metadata', 'INSERT')"), 'f');
assert.match(sql('SELECT postgis_version()'), /^3\.6/);
compose('run', '--rm', 'migrate', 'up');
const metadata = sql('SELECT created_at FROM bootstrap_metadata');
compose('--profile', 'demo', 'run', '--rm', 'seed');
const seed = sql("SELECT applied_at FROM seed_runs WHERE name = 'bootstrap-v1'");
compose('--profile', 'demo', 'run', '--rm', 'seed');
assert.equal(sql("SELECT count(*) FROM seed_runs WHERE name = 'bootstrap-v1'"), '1');
assert.equal(sql("SELECT applied_at FROM seed_runs WHERE name = 'bootstrap-v1'"), seed);

// Exercise dependency failure while the API process stays alive, then restore it even on failure.
try {
  compose('stop', 'postgres');
  const unavailable = await fetch(`${base}/api/v1/health/ready`, { signal: AbortSignal.timeout(5000) });
  assert.equal(unavailable.status, 503);
  const error = await unavailable.json();
  assert.equal(error.code, 'SERVICE_UNAVAILABLE');
  assert.equal(error.request_id, unavailable.headers.get('x-request-id'));
  assert.equal((await fetch(`${base}/api/v1/health/live`)).status, 200);
} finally { compose('start', 'postgres'); }
await ready();
assert.equal(sql('SELECT created_at FROM bootstrap_metadata'), metadata);
assert.equal(sql("SELECT applied_at FROM seed_runs WHERE name = 'bootstrap-v1'"), seed);
console.log(
  'PASS: frontend/proxy, live API/PostGIS, runtime role,',
  'repeated migrations/seed, outage recovery and persistent data.',
);
