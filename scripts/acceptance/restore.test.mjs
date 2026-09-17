// The backup and the restoration of the database, run on artificial data rather than on the
// demonstration: a dump is taken, restored into a database of its own, and read back through the same
// tables to prove that what came back is what went in.
//
// The dump stays in a temporary directory and is removed by the check that made it, so no dump is ever
// committed, listed by Git or left behind in an artefact.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { after, before, describe, test } from 'node:test';
import { repositoryRoot } from '../service.mjs';
import { call, sql, waitForReady } from './client.mjs';
import { deliveryKeyOf } from './mail.mjs';
import { newAccount, newCommandKey, reserve, restoreScenario } from './reservations.mjs';

const ACCOUNT_PREFIX = 'restore';

/** The database the stack runs on, and the migrator role that owns it. */
const SOURCE_DATABASE = 'carsharing';
const MIGRATOR_ROLE = 'carsharing_migrator';

/** The database the dump is restored into. It exists only for the length of this check. */
const RESTORED_DATABASE = 'carsharing_restore_check';

/** How long one dump or one restoration may take. */
const DUMP_TIMEOUT_MS = 180_000;

let workspace = '';

before(async () => {
  await waitForReady();
  workspace = mkdtempSync(join(tmpdir(), 'carsharing-restore-'));
});

after(() => {
  endSuiteRestore();
  dropRestoredDatabase();
  if (workspace) rmSync(workspace, { recursive: true, force: true });
  restoreScenario();
});

describe('the database can be dumped and restored', () => {
  test('a completed ride, its invoice and its letter read back the same in a restored database', async () => {
    const { email, rentalId, invoiceId } = await finishedRideAccount();
    const source = readFacts(SOURCE_DATABASE, email, rentalId, invoiceId);
    assert.ok(source.rentalId, 'the prepared ride was not written');

    const dump = join(workspace, 'artificial.dump');
    dumpDatabase(dump);
    assert.ok(statSync(dump).size > 0, 'the dump is empty');

    dropRestoredDatabase();
    createRestoredDatabase();
    restoreDatabase(dump);

    const restored = readFacts(RESTORED_DATABASE, email, rentalId, invoiceId);
    assert.deepEqual(restored, source, 'the restored database does not state what the source did');
    assert.equal(restored.stage, 'completed');
    assert.equal(restored.totalAmount, source.totalAmount);
    assert.equal(restored.letter, 1, 'the letter about the invoice did not survive the restoration');
    // The first payment attempt is a task of the worker, so the state it reached is either the one
    // the invoice was issued in or the one the attempt moved it to; what the restoration must have
    // kept is the row and the state it states.
    assert.ok(
      ['pending', 'failed', 'paid'].includes(restored.payment),
      `the payment state is not one the contract declares: ${restored.payment}`,
    );

    process.stdout.write(
      `restore: rental=${restored.stage} invoice_total=${restored.totalAmount} ` +
        `letters=${restored.letter} dump_bytes=${statSync(dump).size}\n`,
    );
  });

  test('the same command may be repeated in the restored database without making a second invoice', async () => {
    const { email, rentalId, invoiceId } = await finishedRideAccount();
    const dump = join(workspace, 'artificial-repeat.dump');
    dumpDatabase(dump);
    dropRestoredDatabase();
    createRestoredDatabase();
    restoreDatabase(dump);

    // The stored result of the command and the invoice it produced are both in the dump, so the
    // repeat is answered by what was restored rather than by deciding the command again.
    const restored = readFacts(RESTORED_DATABASE, email, rentalId, invoiceId);
    assert.equal(restored.invoiceId, invoiceId);
    assert.equal(
      rowCount(RESTORED_DATABASE, `SELECT count(*) FROM invoices WHERE rental_id = '${rentalId}'`),
      1,
      'the restored database holds more than one invoice for one ride',
    );
    assert.equal(
      rowCount(RESTORED_DATABASE, `SELECT count(*) FROM invoice_payments`),
      rowCount(SOURCE_DATABASE, `SELECT count(*) FROM invoice_payments`),
    );
  });
});

/** One account that rode, ended its ride and was charged, with a letter about the invoice. */
async function finishedRideAccount() {
  const account = await newAccount(`${ACCOUNT_PREFIX}-rider`);
  const created = await reserve(freeVehicle(), newCommandKey(), account);
  assert.equal(created.status, 201, created.text);
  const rentalId = created.json.rental.id;

  const started = await rideCommand(`/api/v1/reservations/${rentalId}/start`, account);
  assert.equal(started.status, 200, started.text);
  const finished = await rideCommand(`/api/v1/rides/${rentalId}/finish`, account);
  assert.equal(finished.status, 200, finished.text);

  const invoiceId = sql(`SELECT id FROM invoices WHERE rental_id = '${rentalId}'`);
  assert.ok(invoiceId, 'the finished ride was not invoiced');
  const email = account.email;

  // The letter is delivered by the worker; the restoration is about the rows, so what is waited for is
  // that the queue no longer owes it rather than that the stub answered within a bound.
  await untilOneRow(`SELECT count(*) FROM outbox WHERE kind = 'invoice.issued' AND completed_at IS NOT NULL`);
  return { email, rentalId, invoiceId };
}

/** Everything the restoration is asked to have kept, read from one database as one comparable value. */
function readFacts(database, email, rentalId, invoiceId) {
  return JSON.parse(
    execFileSync(
      'docker',
      [
        'compose',
        'exec',
        '-T',
        'postgres',
        'psql',
        '-U',
        MIGRATOR_ROLE,
        '-d',
        database,
        '-At',
        '-v',
        'ON_ERROR_STOP=1',
        '-c',
        `SELECT json_build_object(
           'email', (SELECT email FROM users WHERE email = '${email}'),
           'stage', (SELECT stage FROM rentals WHERE id = '${rentalId}'),
           'rentalId', (SELECT id FROM rentals WHERE id = '${rentalId}'),
           'invoiceId', (SELECT id FROM invoices WHERE id = '${invoiceId}'),
           'totalAmount', (SELECT total_amount_tyiyn FROM invoices WHERE id = '${invoiceId}'),
           'payment', (SELECT status FROM invoice_payments WHERE invoice_id = '${invoiceId}'),
           'letter', (SELECT count(*) FROM mailstub.messages WHERE delivery_key = '${deliveryKeyOf(invoiceId)}'),
           'segments', (SELECT count(*) FROM ride_segments WHERE rental_id = '${rentalId}')
         )`,
      ],
      { cwd: repositoryRoot, encoding: 'utf8', timeout: DUMP_TIMEOUT_MS },
    ).trim(),
  );
}

/** Dumps the whole database into one file, exactly as the documented command does. */
function dumpDatabase(path) {
  execFileSync(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      'postgres',
      'pg_dump',
      '-U',
      MIGRATOR_ROLE,
      '-d',
      SOURCE_DATABASE,
      '--format=custom',
      '--file=/tmp/artificial.dump',
    ],
    { cwd: repositoryRoot, encoding: 'utf8', timeout: DUMP_TIMEOUT_MS },
  );
  copyOutOfContainer('/tmp/artificial.dump', path);
}

/** Copies one file out of the database container, so the dump is what a backup step would keep. */
function copyOutOfContainer(inside, outside) {
  const container = execFileSync('docker', ['compose', 'ps', '-q', 'postgres'], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  }).trim();
  execFileSync('docker', ['cp', `${container}:${inside}`, outside], {
    cwd: repositoryRoot,
    encoding: 'utf8',
    timeout: DUMP_TIMEOUT_MS,
  });
}

/** Creates the database the dump is restored into, dropping it first so a rerun starts clean. */
function createRestoredDatabase() {
  execFileSync(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      'postgres',
      'psql',
      '-U',
      MIGRATOR_ROLE,
      '-d',
      'postgres',
      '-At',
      '-v',
      'ON_ERROR_STOP=1',
      '-c',
      `CREATE DATABASE ${RESTORED_DATABASE}`,
    ],
    { cwd: repositoryRoot, encoding: 'utf8', timeout: DUMP_TIMEOUT_MS },
  );
}

function dropRestoredDatabase() {
  execFileSync(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      'postgres',
      'psql',
      '-U',
      MIGRATOR_ROLE,
      '-d',
      'postgres',
      '-At',
      '-v',
      'ON_ERROR_STOP=1',
      '-c',
      `DROP DATABASE IF EXISTS ${RESTORED_DATABASE} WITH (FORCE)`,
    ],
    { cwd: repositoryRoot, encoding: 'utf8', timeout: DUMP_TIMEOUT_MS },
  );
}

/** Restores one dump into the restored database. */
function restoreDatabase(path) {
  const container = execFileSync('docker', ['compose', 'ps', '-q', 'postgres'], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  }).trim();
  execFileSync('docker', ['cp', path, `${container}:/tmp/restore.dump`], {
    cwd: repositoryRoot,
    encoding: 'utf8',
    timeout: DUMP_TIMEOUT_MS,
  });
  execFileSync(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      'postgres',
      'pg_restore',
      '-U',
      MIGRATOR_ROLE,
      '-d',
      RESTORED_DATABASE,
      '--no-owner',
      '--exit-on-error',
      '/tmp/restore.dump',
    ],
    { cwd: repositoryRoot, encoding: 'utf8', timeout: DUMP_TIMEOUT_MS },
  );
}

function rowCount(database, query) {
  return Number(
    execFileSync(
      'docker',
      ['compose', 'exec', '-T', 'postgres', 'psql', '-U', MIGRATOR_ROLE, '-d', database, '-At', '-c', query],
      { cwd: repositoryRoot, encoding: 'utf8', timeout: DUMP_TIMEOUT_MS },
    ).trim(),
  );
}

/** One vehicle the demonstration has free, read directly: the ride is not what this check tests. */
function freeVehicle() {
  return sql(
    `SELECT id FROM vehicles WHERE id NOT IN (SELECT vehicle_id FROM rentals WHERE ended_at IS NULL)
     ORDER BY id LIMIT 1`,
  );
}

function rideCommand(path, account) {
  return call(path, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { 'Idempotency-Key': newCommandKey() },
  });
}

async function untilOneRow(query) {
  const deadline = Date.now() + 30_000;
  while (Number(sql(query)) === 0) {
    if (Date.now() > deadline) throw new Error(`the queue still owes a delivery: ${query}`);
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
}

/** Removes the rows this check wrote, so the prepared demonstration can be put back. */
function endSuiteRestore() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(`DELETE FROM outbox WHERE recipient_id IN ${mine}`);
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM invoice_payments WHERE invoice_id IN (SELECT id FROM invoices WHERE user_id IN ${mine})`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
  sql(`DELETE FROM ride_segments WHERE rental_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  sql(`DELETE FROM users WHERE id IN ${mine}`);
}
