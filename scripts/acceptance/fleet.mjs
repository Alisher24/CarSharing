// What the fleet suites read the demonstration through: the public paths, the SQL the database
// answers a geometry question with, and the compose commands that install and restore the scenario.
import { spawn } from 'node:child_process';
import { composeWith, POSTGRES_DATABASE, POSTGRES_ROLE, POSTGRES_SERVICE, repositoryRoot, sql } from '../service.mjs';

export const VEHICLES_PATH = '/api/v1/vehicles';
export const ZONES_PATH = '/api/v1/zones';
export const TARIFFS_PATH = '/api/v1/tariffs';

/** The demonstration promises five vehicles of each of five powertrains. */
export const POWERTRAIN_TYPES = ['electric', 'gasoline', 'diesel', 'hybrid', 'gas'];
export const VEHICLES_PER_POWERTRAIN = 5;
export const FLEET_SIZE = POWERTRAIN_TYPES.length * VEHICLES_PER_POWERTRAIN;

/** The state a group of five starts in, which is what makes every public state visible at once. */
export const STATES_PER_POWERTRAIN = { available: 2, reserved: 1, in_trip: 1, unavailable: 1 };

/** The reserve one source must hold on its own for a rental to start. */
export const START_THRESHOLD_BASIS_POINTS = 2000;

/** How old a confirmed position may be and still be trusted. */
export const MAX_TELEMETRY_AGE_SECONDS = 15;

/** The demonstration prices, in whole tyiyn per started minute of each mode. */
export const DRIVING_RATE_TYIYN = 1234;
export const PAUSED_RATE_TYIYN = 321;

const DEMO_COMPOSE_PROFILE = 'demo';

/** Runs one of the demonstration's own one-off commands, as an operator would. */
function runDemoService(service) {
  return composeWith({}, '--profile', DEMO_COMPOSE_PROFILE, 'run', '--rm', service);
}

export function runSeed() {
  return runDemoService('seed');
}

export function restoreScenario() {
  return runDemoService('demo-scenario');
}

/** Reads one value from the database, which is how a geometry question is put to PostGIS itself. */
export function scalar(query) {
  return sql(query);
}

/** Reads a query that returns one column and many rows. */
export function column(query) {
  const answer = sql(query);
  return answer === '' ? [] : answer.split('\n');
}

/** A point written the way PostGIS reads one: longitude first, in WGS84. */
export function pointAt(longitude, latitude) {
  return `ST_SetSRID(ST_MakePoint(${longitude}, ${latitude}), 4326)`;
}

/** Every energy source of the fleet, as the catalog publishes them. */
export function everySource(vehicles) {
  return vehicles.flatMap((vehicle) => vehicle.energy_sources);
}

export function vehiclesOfPowertrain(vehicles, powertrain) {
  return vehicles.filter((vehicle) => vehicle.powertrain_type === powertrain);
}

export function countStates(vehicles) {
  const counted = { available: 0, reserved: 0, in_trip: 0, unavailable: 0 };
  for (const vehicle of vehicles) counted[vehicle.status] += 1;
  return counted;
}

/**
 * Runs the restoration and reports whether it went ahead. A refusal is an outcome these suites
 * assert about rather than a failure of the harness, so the command's own message is returned with
 * it.
 */
export function tryRestoreScenario() {
  try {
    return { ok: true, output: restoreScenario() };
  } catch (failure) {
    return { ok: false, output: `${failure.stdout ?? ''}${failure.stderr ?? ''}` };
  }
}

/**
 * Holds one database transaction open for a while, so a suite can start a second command against
 * the rows it has locked. The promise settles when the transaction has committed.
 */
export function holdTransaction(statements, heldSeconds) {
  const query = `BEGIN; ${statements} SELECT pg_sleep(${heldSeconds}); COMMIT;`;
  const psql = spawn(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      POSTGRES_SERVICE,
      'psql',
      '-U',
      POSTGRES_ROLE,
      '-d',
      POSTGRES_DATABASE,
      '-v',
      'ON_ERROR_STOP=1',
      '-c',
      query,
    ],
    { cwd: repositoryRoot, stdio: ['ignore', 'pipe', 'pipe'] },
  );
  return new Promise((settle, fail) => {
    let errors = '';
    psql.stderr.on('data', (chunk) => {
      errors += chunk;
    });
    psql.on('error', fail);
    psql.on('close', (code) => (code === 0 ? settle() : fail(new Error(errors))));
  });
}
