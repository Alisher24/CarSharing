// Who may reach each operation of the public contract, observed on the assembled stack. The matrix
// is derived from the contract's own served set, so this suite is about the rule rather than about a
// list of operations somebody remembered to write down: a foreign identifier must be answered exactly
// as an absent one, and the rows of the account that was aimed at must not move.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import { ABSENT_IDENTIFIER, OWNED, PUBLIC, SESSION, answerShape, accessMatrix, pathFor, sendTo } from './access.mjs';
import { issueCursor, positionOf } from './cursors.mjs';
import { insertCompletedRide } from './history.mjs';
import { insertNotification } from './notifications.mjs';
import { insertRental } from './rentalrows.mjs';
import { call, newEmail, registerAccount, resetRateLimits, signInFromSecondDevice, waitForReady } from './client.mjs';
import {
  IDEMPOTENCY_HEADER,
  availableVehicle,
  availableVehicles,
  endSuiteReservations,
  newAccount,
  newCommandKey,
  reserve,
  restoreScenario,
  sql,
} from './reservations.mjs';

/** Every account this suite registers carries this prefix, which is how its rows are found again. */
const ACCOUNT_PREFIX = 'access';

const STATUS_AUTHENTICATION_REQUIRED = 401;
const STATUS_NOT_FOUND = 404;
const STATUS_OK = 200;
const STATUS_NO_CONTENT = 204;
const STATUS_BAD_REQUEST = 400;
const STATUS_UNPROCESSABLE = 422;
const STATUS_CREATED = 201;
const NOT_FOUND_CODE = 'RESOURCE_NOT_FOUND';
const AUTHENTICATION_REQUIRED_CODE = 'AUTHENTICATION_REQUIRED';
const INVALID_CURSOR_CODE = 'INVALID_CURSOR';

/** How many rows of the account under attack are compared before and after a foreign request. */
const OWNED_TABLES = ['rentals', 'invoices', 'invoice_payments', 'notifications'];

before(waitForReady);

// The suite needs a live ride to aim foreign commands at, so what one check left holding is given
// back before the next starts and the prepared demonstration is put back at the end.
beforeEach(endSuiteReservations);
// The prepared demonstration is put back after the rows this suite wrote are gone: the restoration
// refuses while a rental of a person's — ended or not — stands on one of its vehicles. The suite
// writes rows directly, so it removes them directly rather than asking the service to release them.
after(() => {
  endSuiteAccess();
  restoreScenario();
});

describe('an operation without a session', () => {
  test('answers 401 and no data of any account', async () => {
    resetRateLimits();
    for (const operation of accessMatrix().filter((one) => one.access !== PUBLIC)) {
      const answer = await sendTo(operation, ABSENT_IDENTIFIER);
      assert.equal(
        answer.status,
        STATUS_AUTHENTICATION_REQUIRED,
        `${operation.operationId} without a session answered ${answer.status}: ${answer.text}`,
      );
      assert.equal(answer.json.code, AUTHENTICATION_REQUIRED_CODE, operation.operationId);
      assert.ok(
        !answer.text.includes('@example.test'),
        `${operation.operationId} named an account to a caller without a session: ${answer.text}`,
      );
    }
  });
});

describe('the operations that are open to anyone', () => {
  test('answer without a session, and declare nothing of an account', async () => {
    resetRateLimits();
    const readable = accessMatrix().filter((one) => one.access === PUBLIC && one.operationId !== 'getPublicEvents');
    for (const operation of readable) {
      const answer = await sendTo(operation, ABSENT_IDENTIFIER);
      // A read of the catalog, a probe, a refusal of an unknown vehicle and the two account
      // operations an anonymous caller may send: each is an answer that names no account.
      assert.ok(
        [STATUS_OK, STATUS_NO_CONTENT, STATUS_BAD_REQUEST, STATUS_NOT_FOUND, STATUS_UNPROCESSABLE].includes(
          answer.status,
        ),
        `${operation.operationId} answered ${answer.status}: ${answer.text}`,
      );
      assert.ok(
        !answer.text.includes('@example.test'),
        `${operation.operationId} answered with an account's data: ${answer.text}`,
      );
    }
  });

  test('an absent vehicle is a 404 that names no vehicle', async () => {
    const absent = await call(
      pathFor(
        accessMatrix().find((one) => one.operationId === 'getVehicle'),
        ABSENT_IDENTIFIER,
      ),
    );
    assert.equal(absent.status, STATUS_NOT_FOUND, absent.text);
    assert.equal(absent.json.code, NOT_FOUND_CODE);
  });
});

describe('another account may not reach an object it does not hold', () => {
  test('every operation on an object answers a foreign identifier as an absent one', async () => {
    resetRateLimits();
    const [ownersVehicle] = await availableVehicles(1);
    const { account: owner, rentalId, invoiceId, notificationId } = await accountWithObjects('matrix', ownersVehicle);
    const stranger = await newAccount(`${ACCOUNT_PREFIX}-stranger`);
    const identifiers = { rental: rentalId, invoice: invoiceId, notification: notificationId };
    const checked = [];

    for (const operation of accessMatrix().filter((one) => one.access === OWNED)) {
      const identifier = identifiers[operation.object];
      assert.ok(identifier, `${operation.operationId} has no ${operation.object} to aim at`);

      // A pause, a resume and a finish act on a ride that stands in a state the command accepts, so
      // the owner's ride is left where the operation can decide it: a refusal that came from the
      // state of the ride rather than from the account would prove nothing about the account.
      const before = ownRows(owner, operation.object);
      const foreign = await sendTo(operation, identifier, { account: stranger, key: newCommandKey() });
      const absent = await sendTo(operation, ABSENT_IDENTIFIER, { account: stranger, key: newCommandKey() });
      const after = ownRows(owner, operation.object);

      // A foreign identifier and an identifier that does not exist are one answer: the difference
      // would tell the caller that the first one exists.
      assert.equal(
        foreign.status,
        STATUS_NOT_FOUND,
        `${operation.operationId} answered a foreign ${operation.object} with ${foreign.status}: ${foreign.text}`,
      );
      assert.equal(
        foreign.json.code,
        NOT_FOUND_CODE,
        `${operation.operationId} answered a foreign ${operation.object} with ${foreign.json.code}`,
      );
      assert.deepEqual(
        answerShape(foreign),
        answerShape(absent),
        `${operation.operationId} told a foreign identifier apart from an absent one`,
      );
      assert.ok(
        !foreign.text.includes(rentalId) && !foreign.text.includes(invoiceId) && !foreign.text.includes(notificationId),
        `${operation.operationId} answered with the owner's object: ${foreign.text}`,
      );
      // The refusal must be a refusal: nothing of the account that was aimed at may have moved.
      assert.equal(
        after,
        before,
        `${operation.operationId} changed the owner's ${operation.object} while refusing the request`,
      );
      checked.push(operation.operationId);
    }

    assert.equal(checked.length, accessMatrix().filter((one) => one.access === OWNED).length);
    process.stdout.write(`access matrix: ${checked.length} owned operations refused a foreign identifier\n`);
  });

  test('a correct command of the owner still works, so the refusal is about ownership', async () => {
    resetRateLimits();
    const { account: owner, rentalId } = await accountWithObjects('owner-works');
    const cancelled = await call(`/api/v1/reservations/${rentalId}/cancel`, {
      method: 'POST',
      cookie: owner.cookie,
      csrfToken: owner.csrfToken,
      headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
    });
    assert.equal(cancelled.status, STATUS_OK, cancelled.text);
    assert.equal(cancelled.json.rental.id, rentalId);
  });
});

describe('a cursor belongs to one account, one operation and one set of parameters', () => {
  // The cursor of one collection is refused to another scope by the shared signer, which is where
  // the operation, the owner and the parameters are checked. A page has to carry a cursor of its own
  // for that to be read at all, so the account under test is given rows of every collection this
  // check walks; they are written rather than ridden, and the check below is about the cursor.
  test('a cursor of one account is refused by every collection of another', async () => {
    resetRateLimits();
    const owner = await accountWithPages();
    const stranger = await newAccount(`${ACCOUNT_PREFIX}-cursor-stranger`);
    const ownerId = accountId(owner);
    const strangerId = accountId(stranger);
    const collections = [
      { operation: 'getNotifications', path: '/api/v1/me/notifications' },
      { operation: 'getRides', path: '/api/v1/me/rides' },
      { operation: 'getInvoices', path: '/api/v1/me/invoices' },
    ];
    let checked = 0;

    for (const collection of collections) {
      // A page of the account that holds something, so the answer carries a cursor of its own.
      const first = await call(`${collection.path}?limit=1`, { cookie: owner.cookie });
      assert.equal(first.status, STATUS_OK, first.text);
      assert.ok(first.json.next_cursor, `${collection.operation} answered no cursor for a page with more to read`);
      const { createdAt, id } = positionOf(first.json.next_cursor);
      const issuedFor = { operation: collection.operation, owner: ownerId, params: { limit: '1' } };

      const sameScope = await cursorCall(collection.path, issueCursor(issuedFor, { createdAt, id }), owner);
      assert.equal(
        sameScope.status,
        STATUS_OK,
        `the cursor of a page was refused to its own reader: ${sameScope.text}`,
      );

      const otherAccount = await cursorCall(
        collection.path,
        issueCursor({ ...issuedFor, owner: strangerId }, { createdAt, id }),
        owner,
      );
      assert.equal(otherAccount.status, 400, otherAccount.text);
      assert.equal(otherAccount.json.code, INVALID_CURSOR_CODE, otherAccount.text);

      const otherOperation = collections.find((one) => one.path !== collection.path);
      const acrossOperations = await cursorCall(otherOperation.path, issueCursor(issuedFor, { createdAt, id }), owner);
      assert.equal(acrossOperations.status, 400, acrossOperations.text);
      assert.equal(acrossOperations.json.code, INVALID_CURSOR_CODE, acrossOperations.text);
      checked += 1;
    }

    assert.equal(checked, collections.length, 'not every collection was checked');
    process.stdout.write(`access matrix: ${checked} collections refused a cursor of another scope\n`);
  });
});

describe('a stored command result belongs to the account that made the command', () => {
  test('another account with the same key never receives the stored body', async () => {
    resetRateLimits();
    const owner = await newAccount(`${ACCOUNT_PREFIX}-key-owner`);
    const stranger = await newAccount(`${ACCOUNT_PREFIX}-key-stranger`);
    const key = newCommandKey();
    const [ownersVehicle, strangersVehicle] = await availableVehicles(2);

    const mine = await reserve(ownersVehicle, key, owner);
    assert.equal(mine.status, 201, mine.text);
    const repeat = await reserve(ownersVehicle, key, owner);
    assert.equal(repeat.headers.get('Idempotency-Replayed'), 'true', repeat.text);

    // The stranger sends the identical request under the owner's key. Whatever the service decides вЂ”
    // a refusal of its own or a command of its own вЂ” the owner's stored answer must not come back.
    const foreign = await reserve(strangersVehicle, key, stranger);
    const behaviour = foreign.status === 409 ? 'refused with a conflict' : `answered ${foreign.status}`;
    process.stdout.write(`idempotency: a foreign key was ${behaviour}\n`);
    assert.ok(
      !foreign.text.includes(mine.json.rental.id),
      `the owner's stored answer reached another account: ${foreign.text}`,
    );
    assert.notEqual(foreign.headers.get('Idempotency-Replayed'), 'true', 'a foreign account was given a repeat');
  });

  test('the owner keeps the stored answer after a new sign-in', async () => {
    resetRateLimits();
    const owner = await newAccount(`${ACCOUNT_PREFIX}-key-session`);
    const key = newCommandKey();
    const [ownersVehicle] = await availableVehicles(1);
    const mine = await reserve(await availableVehicle(), key, owner);
    assert.equal(mine.status, 201, mine.text);

    const secondDevice = await signInFromSecondDevice(owner.email);
    const repeat = await reserve(ownersVehicle, key, secondDevice);
    assert.equal(repeat.status, 201, repeat.text);
    assert.equal(repeat.headers.get('Idempotency-Replayed'), 'true', repeat.text);
    assert.deepEqual(repeat.json, mine.json);
  });
});

describe('an unusable email is not a way of asking which addresses have accounts', () => {
  // The boundary answers a malformed address before the operation sees it, so a caller can tell it
  // from an address no account holds by the status alone: 422 against 401, where the operation's own
  // answer is 401 for both. The suite records the difference rather than demanding a behaviour the
  // contract does not have; the finding is named in the report of this audit.
  test('records how a malformed address is answered', async (context) => {
    resetRateLimits();
    const malformed = await call('/api/v1/auth/login', {
      method: 'POST',
      body: { email: 'not-an-address', password: 'correcthorsebattery' },
    });
    const unknown = await call('/api/v1/auth/login', {
      method: 'POST',
      body: { email: newEmail('never-registered'), password: 'correcthorsebattery' },
    });
    process.stdout.write(
      `malformed address: ${malformed.status} ${malformed.json?.code ?? ''} against ` +
        `${unknown.status} ${unknown.json?.code ?? ''} for an address no account holds\n`,
    );
    context.todo('the boundary answers a malformed address as a validation failure and an unknown one as 401');
  });

  test('answers a registered and an unregistered address alike and in comparable time', async () => {
    resetRateLimits();
    const known = await registerAccount('timing');
    resetRateLimits();

    const startedKnown = performance.now();
    await call('/api/v1/auth/login', { method: 'POST', body: { email: known.email, password: 'wrongpasswordvalue' } });
    const knownDuration = performance.now() - startedKnown;

    const startedUnknown = performance.now();
    await call('/api/v1/auth/login', {
      method: 'POST',
      body: { email: newEmail('unknown'), password: 'wrongpasswordvalue' },
    });
    const unknownDuration = performance.now() - startedUnknown;

    process.stdout.write(
      `sign-in timing: registered=${knownDuration.toFixed(1)}ms unregistered=${unknownDuration.toFixed(1)}ms\n`,
    );
    // Both verify a memory-hard hash, so the two answers are of one order; a lookup that skipped the
    // hash would answer the unknown address in a fraction of the time.
    const ratio = knownDuration / Math.max(unknownDuration, 1);
    assert.ok(ratio > 0.25 && ratio < 4, `the two answers differed by a factor of ${ratio.toFixed(2)}`);
  });
});

/** One account holding a rental, its invoice and one notification, which foreign requests are aimed at. */
async function accountWithObjects(name, vehicle) {
  const account = await newAccount(`${ACCOUNT_PREFIX}-${name}`);
  const created = await reserve(vehicle ?? (await availableVehicle()), newCommandKey(), account);
  assert.equal(created.status, 201, created.text);
  const rentalId = created.json.rental.id;
  const owner = accountId(account);

  // A second, completed rental of the same account on another vehicle: one vehicle carries one live
  // rental, and a ride that ended is a row of its own alongside the reservation that stands.
  const [completedVehicle] = await availableVehicles(1);

  // A second rental of the same account, completed with an invoice and a live notification, so that
  // an invoice and a notification of this account exist to be aimed at. They are written rather than
  // ridden for: the commands that produce them are proved by the suites that send them, and this one
  // is about who may read them afterwards. The rental carries the conditions of the price list in
  // force, which is what any rental row states.
  sql(
    insertRental({
      id: sql('SELECT uuidv7()'),
      email: account.email,
      vehicleId: completedVehicle,
      stage: 'completed',
      reservedAt: `now() - interval '2 hours'`,
      expiresAt: `now() - interval '2 hours' + interval '15 minutes'`,
      startedAt: `now() - interval '2 hours'`,
      endedAt: `now() - interval '1 hour'`,
      completionReason: 'user_finished',
    }),
  );
  const completedRentalId = sql(
    `SELECT id FROM rentals WHERE user_id = '${owner}' AND stage = 'completed' ORDER BY ended_at DESC LIMIT 1`,
  );
  sql(
    `INSERT INTO invoices (id, rental_id, user_id, issued_at, currency, billing_policy, completion_reason,
                           driving_duration_microseconds, paused_duration_microseconds,
                           driving_billed_started_minutes, paused_billed_started_minutes,
                           driving_rate_tyiyn_per_started_minute, paused_rate_tyiyn_per_started_minute, total_amount_tyiyn, version)
     SELECT uuidv7(), '${completedRentalId}', '${owner}', now() - interval '1 hour', 'KGS',
            'per_mode_started_minute_v1', 'user_finished', 120000000, 0, 2, 0, 1234, 321, 2468, 1`,
  );
  const invoiceId = sql(`SELECT id FROM invoices WHERE rental_id = '${completedRentalId}'`);
  sql(
    `INSERT INTO notifications (id, user_id, rental_id, kind, created_at, active, version)
     VALUES (uuidv7(), '${owner}', '${rentalId}', 'reservation_expiring', now(), true, 1)`,
  );
  const notificationId = sql(
    `SELECT id FROM notifications WHERE user_id = '${owner}' ORDER BY created_at DESC LIMIT 1`,
  );

  return { account, rentalId, invoiceId, notificationId, completedRentalId };
}

/**
 * One account whose collections each hold more than one row, so a page of one carries a cursor. The
 * three collections are listed newest first, and the cursor check reads the moment and the
 * identifier a page was cut at rather than the rows themselves, so two rows of each are written for
 * it directly: the commands that produce a ride, an invoice and a notification are proved by the
 * suites that send them.
 */
async function accountWithPages() {
  const account = await newAccount(`${ACCOUNT_PREFIX}-cursor-owner`);

  insertCompletedRide({ account, endedSecondsAgo: 7200 });
  insertCompletedRide({ account, endedSecondsAgo: 3600 });
  insertNotification({ account, createdSecondsAgo: 300 });
  insertNotification({ account, createdSecondsAgo: 100 });

  return account;
}

/** The rows of one account that a foreign request must leave exactly as they were. */
function ownRows(account, object) {
  const owner = accountId(account);
  const table = { rental: 'rentals', invoice: 'invoices', notification: 'notifications' }[object];
  assert.ok(OWNED_TABLES.includes(table), `no snapshot is defined for ${object}`);
  const rows = sql(`SELECT coalesce(md5(string_agg(t::text, '' ORDER BY t::text)), 'empty') FROM ${table} t
                    WHERE user_id = '${owner}'`);
  if (table !== 'invoices') return rows;
  // An invoice carries the payment that settles it, so a foreign payment request could move a row of
  // a table the invoice itself does not name.
  return `${rows}:${sql(
    `SELECT coalesce(md5(string_agg(p::text, '' ORDER BY p::text)), 'empty')
     FROM invoice_payments p JOIN invoices i ON i.id = p.invoice_id WHERE i.user_id = '${owner}'`,
  )}`;
}

function cursorCall(path, cursor, account) {
  return call(`${path}?limit=1&cursor=${encodeURIComponent(cursor)}`, { cookie: account.cookie });
}

/** The account's own identifier, which a check needs to write rows or sign a cursor for it. */
function accountId(account) {
  const owner = sql(`SELECT id FROM users WHERE email = '${account.email}'`);
  assert.ok(owner, `the account ${account.email} was never registered, so this check cannot continue`);
  return owner;
}

/** Ends what this suite made, so the prepared demonstration can be put back. */
function endSuiteAccess() {
  const mine = `(SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`;
  sql(`DELETE FROM outbox WHERE resource_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(`DELETE FROM outbox WHERE resource_id IN (SELECT id FROM invoices WHERE user_id IN ${mine})`);
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM invoice_payments WHERE invoice_id IN (SELECT id FROM invoices WHERE user_id IN ${mine})`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
  sql(`DELETE FROM outbox WHERE recipient_id IN ${mine}`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  sql(`DELETE FROM users WHERE id IN ${mine}`);
  sql(
    `DELETE FROM rate_limit_counters WHERE subject LIKE '${ACCOUNT_PREFIX}-%' OR subject LIKE '%${ACCOUNT_PREFIX}-%'`,
  );
}
