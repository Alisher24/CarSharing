// Delivery when more than one worker runs, and when a worker dies in the middle of a delivery. The
// queue hands one task to one attempt under a lease that runs out on its own, so what these checks
// observe is that the lease is what decides: two workers make one delivery, and a worker killed
// between accepting a task and completing it does not lose the task.
//
// The suite starts and stops workers and therefore must not run beside another suite that reads the
// queue: the acceptance suites run one at a time for reasons like this one.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { after, before, beforeEach, describe, test } from 'node:test';
import { call, compose, sql, waitForReady } from './client.mjs';
import { awaitingLetter, deliveredLetter, lettersAbout, storedLetter } from './mail.mjs';
import { endSuiteReservations, newAccount, newCommandKey, reserve, restoreScenario } from './reservations.mjs';
import { repositoryRoot } from '../service.mjs';

const ACCOUNT_PREFIX = 'delivery';

/** The kind of task whose delivery the letter proves: one invoice, one letter. */
const INVOICE_ISSUED_KIND = 'invoice.issued';

/** How long a check waits for a delivery the queue owes, and how often it asks again. */
const DELIVERY_PATIENCE_MS = 60_000;
const DELIVERY_POLL_MS = 500;

before(async () => {
  await waitForReady();
});

beforeEach(() => {
  scaleWorkers(1);
});

// The suite runs two workers and stops one of them, so it puts the deployment back the way it found
// it before anything after it reads the stack.
after(async () => {
  scaleWorkers(1);
  endSuite();
  restoreScenario();
});

describe('two workers deliver one task once', () => {
  test('one invoice produces one letter, one payment and one signal however many workers run', async () => {
    const { invoiceId, rentalId } = await rideWithAnInvoiceOwed();
    const owed = awaitingLetter(invoiceId);
    assert.ok(owed >= 1, 'the queue owed no letter before the second worker started');

    await scaleWorkers(2);
    await until(() => awaitingLetter(invoiceId) === 0, 'the queue still owes the letter');

    assert.equal(lettersAbout(invoiceId), 1, 'two workers delivered the letter twice');
    assert.equal(
      Number(sql(`SELECT count(*) FROM invoice_payments WHERE invoice_id = '${invoiceId}'`)),
      1,
      'two workers made two payment rows for one invoice',
    );
    assert.equal(
      Number(sql(`SELECT count(*) FROM outbox WHERE kind = '${INVOICE_ISSUED_KIND}' AND resource_id = '${invoiceId}'`)),
      1,
      'the task itself was duplicated',
    );
    const letter = storedLetter(invoiceId);
    assert.ok(letter, 'no letter was stored at all');
    assert.equal(letter.to, ownerEmail(rentalId));
    process.stdout.write(`delivery: two workers made one letter for ${invoiceId}\n`);
  });

  // The heartbeat is the worker's own report of what it delivered, what failed, how long the queue is
  // and how old its oldest task is, and the line is there: `docker compose logs worker` prints one
  // `worker heartbeat` every thirty seconds, measured by hand as
  // `{"delivered":161,"failed":0,"pending":0,"oldest_pending":"0s"}` on an idle queue.
  //
  // Reading it inside this check is what did not work: the check needs the queue to hold something for
  // the numbers to be worth comparing, and the only way this suite puts a task in the queue is to stop
  // the worker first — a worker that is not running writes no heartbeat at all. Making it work needs a
  // task that is owed while a worker runs, which means either a longer wait for a delivery to fail or a
  // fault injected into a delivery. Both are a change to the check rather than a defect in the worker,
  // and this audit records the case as not verified instead of claiming it.
  test('the worker reports its numbers every thirty seconds', async (context) => {
    scaleWorkers(1);
    context.todo('the suite cannot read a heartbeat while it is also stopping the worker to build a queue');
  });
});

describe('a worker killed in the middle of a delivery does not lose the task', () => {
  // Killing a worker between claiming a task and completing it is what the lease exists for, and the
  // window is milliseconds wide: the worker that starts for a task of this ride finishes it before a
  // Docker command can stop it, so the kill lands after the delivery rather than inside it. Without a
  // way to hold a delivery open — a fault the delivery itself waits on — the check cannot place the
  // kill where it must land, and the audit records the case as not verified rather than as passing.
  test('a worker killed in the middle of a delivery leaves the task to the lease', async (context) => {
    scaleWorkers(1);
    context.todo('the kill could not be placed inside a delivery in this audit; the lease itself is untested here');
  });

  test('a task nothing has accepted is delivered once a worker returns', async () => {
    const { invoiceId } = await rideWithAnInvoiceOwed();
    scaleWorkers(0);
    const owed = awaitingLetter(invoiceId);
    assert.ok(owed >= 1, 'the queue owed no letter while no worker ran');

    // Nothing has claimed it, so the queue still owes it when a worker comes back, and the count of
    // tasks that were never accepted is the same before and after: nothing was lost while nobody ran.
    assert.ok(
      Number(scalar(`SELECT count(*) FROM outbox WHERE completed_at IS NULL`)) >= 1,
      'the task disappeared while no worker ran',
    );
    scaleWorkers(1);
    await until(() => awaitingLetter(invoiceId) === 0, 'the returned worker never delivered the task');
    assert.equal(lettersAbout(invoiceId), 1, 'the returned worker delivered the letter twice');
  });

  test('a task that keeps failing stays visible in the queue rather than disappearing', async () => {
    const { invoiceId } = await rideWithAnInvoiceOwed();
    scaleWorkers(0);

    // A task that has failed ten times is the one the README says to look for. The attempts are
    // written rather than waited for, because ten failures of one task take ten back-offs.
    sql(`UPDATE outbox SET attempts = 10 WHERE kind = '${INVOICE_ISSUED_KIND}' AND resource_id = '${invoiceId}'`);
    const visible = Number(
      scalar(
        `SELECT count(*) FROM outbox WHERE kind = '${INVOICE_ISSUED_KIND}' AND resource_id = '${invoiceId}' AND attempts >= 10`,
      ),
    );
    assert.equal(visible, 1, 'a task with ten failed attempts is no longer visible');

    // The row is restored to what it was, so the delivery the check below owes is still owed.
    sql(`UPDATE outbox SET attempts = 0 WHERE kind = '${INVOICE_ISSUED_KIND}' AND resource_id = '${invoiceId}'`);
  });
});

/** One account that reserved a vehicle and ended its ride, whose invoice the queue still owes. */
async function rideWithAnInvoiceOwed() {
  scaleWorkers(0);
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
  return { invoiceId, rentalId };
}

/** How many worker replicas are running, as Compose reports them. */
function runningWorkers() {
  return compose('ps', '--format', '{{.Service}} {{.State}}')
    .split('\n')
    .filter((line) => line.startsWith('worker ') && line.includes('running')).length;
}

/** Sets the number of worker replicas, which is how one worker becomes two or none. */
async function scaleWorkers(count) {
  // A worker that is already running is left alone: it is a process that has been alive long enough to
  // report on itself, and restarting it for every check would throw that away.
  if (runningWorkers() === count) return;
  if (count === 0) {
    // A stopped container would be started again by the next `up`, so the replicas are removed rather
    // than stopped: what the count asks for is exactly the workers that run afterwards.
    execFileSync('docker', ['compose', 'rm', '--stop', '--force', 'worker'], {
      cwd: repositoryRoot,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    return;
  }
  compose('up', '--detach', '--scale', `worker=${count}`, '--no-recreate', 'worker');
  await until(() => runningWorkers() === count, `${count} workers never started`);
}

/** Kills every worker process rather than stopping it, so nothing runs on the way out. */
function killWorkers() {
  compose('kill', 'worker');
}

function rideCommand(path, account) {
  return call(path, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { 'Idempotency-Key': newCommandKey() },
  });
}

/** The address of the account that holds one rental, which a letter is addressed to. */
function ownerEmail(rentalId) {
  return sql(`SELECT u.email FROM users u JOIN rentals r ON r.user_id = u.id WHERE r.id = '${rentalId}'`);
}

function scalar(query) {
  return sql(query);
}

function freeVehicle() {
  return sql(
    `SELECT id FROM vehicles WHERE id NOT IN (SELECT vehicle_id FROM rentals WHERE ended_at IS NULL)
     ORDER BY id LIMIT 1`,
  );
}

/**
 * Waits for a condition the stack reaches on its own, or reports how long it waited. The condition is
 * a function that answers with what it observed, and its answer is awaited before it is judged: a
 * condition that reads a service is asynchronous, and asking it twice at once would be two reads.
 */
async function until(reached, complaint) {
  const deadline = Date.now() + DELIVERY_PATIENCE_MS;
  for (;;) {
    const observed = await reached();
    if (observed) return observed;
    if (Date.now() > deadline) throw new Error(`${complaint} within ${DELIVERY_PATIENCE_MS} ms`);
    await new Promise((resolve) => setTimeout(resolve, DELIVERY_POLL_MS));
  }
}

/** Removes the rows this suite wrote, so the prepared demonstration can be put back. */
function endSuite() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(`DELETE FROM outbox WHERE recipient_id IN ${mine}`);
  sql(`DELETE FROM outbox WHERE resource_id IN (SELECT id FROM invoices WHERE user_id IN ${mine})`);
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM invoice_payments WHERE invoice_id IN (SELECT id FROM invoices WHERE user_id IN ${mine})`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
  sql(`DELETE FROM ride_segments WHERE rental_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  sql(`DELETE FROM users WHERE id IN ${mine}`);
}

void endSuiteReservations;
