// The two collections the cabinet is made of, observed on the real HTTP boundary and in the
// PostgreSQL the running service reads: the pages and their order, every refusal a cursor that
// belongs elsewhere meets, what the history leaves out, and the invoice of another account.
//
// The rows of a known shape are written directly, because a check cannot ride twenty rides. What a
// real ride and a real invoice look like is proved by the suites that send those commands; this one
// is about what the collections publish once the rows exist.
import assert from 'node:assert/strict';
import { after, before, beforeEach, describe, test } from 'node:test';
import { call, waitForReady } from './client.mjs';
import {
  CURSOR_ALPHABET,
  INVOICES_PATH,
  MOMENT,
  RIDES_PATH,
  WRITTEN_TOTAL_TYIYN,
  endSuiteHistory,
  insertCompletedRide,
  insertRental,
  invoiceOf,
  invoicesOf,
  issueCursor,
  newSuiteAccount,
  ownerOf,
  positionOf,
  publishedInvoiceOrder,
  publishedRideOrder,
  ridesOf,
  sql,
  tamper,
} from './history.mjs';
import { availableVehicles, endSuiteReservations, restoreScenario } from './reservations.mjs';

/** How many records a page of two holds, which is the smallest page that has an order to check. */
const PAGE_SIZE = 2;

/** How many rides the paging checks write, which is one more than two pages of two hold. */
const WRITTEN_RIDES = 3;

/**
 * The moment the rows of this suite are measured from, read once from the database clock as the
 * literal a query can state it as. It is read once because both collections are ordered by a moment,
 * and two rows written one after another would otherwise differ by milliseconds in the opposite
 * direction to the order the check meant to write them in.
 */
let referenceMoment = '';

/** Writes one finished ride and its invoice, a stated number of seconds before that moment. */
function rideEndedAgo(account, seconds) {
  return insertCompletedRide({ account, at: `'${referenceMoment}'::timestamptz`, endedSecondsAgo: seconds });
}

/** The identifiers one page published, in the order it published them. */
function identifiersOf(page) {
  return page.items.map((item) => item.id);
}

/** The invoices one page published, in the order it published them. */
function invoiceIdentifiersOf(page) {
  return page.items.map((item) => item.invoice.id);
}

before(async () => {
  await waitForReady();
  referenceMoment = sql('SELECT clock_timestamp()');
});

// A check that left a rental behind would hold a vehicle the next one needs, and the suites after
// this one read the prepared demonstration.
beforeEach(endSuiteHistory);

after(async () => {
  await endSuiteHistory();
  await endSuiteReservations();
  await restoreScenario();
});

describe('the history of finished rides', () => {
  test('publishes one page, newest first, with what a ride states about itself', async () => {
    const account = await newSuiteAccount('rides-page');
    const { rentalId, invoiceId } = rideEndedAgo(account, 30);

    const answer = await ridesOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.deepEqual(identifiersOf(answer.json), [rentalId]);
    assert.equal(answer.json.next_cursor, null, 'the only page offered a continuation');

    const [ride] = answer.json.items;
    assert.equal(ride.invoice_id, invoiceId);
    assert.equal(ride.completion.reason, 'user_finished');
    assert.match(ride.started_at, MOMENT);
    assert.match(ride.completed_at, MOMENT);
    assert.equal(typeof ride.vehicle.model, 'string');
    assert.notEqual(ride.vehicle.model, '');
  });

  test('walks the whole history in the order the database holds it', async () => {
    const account = await newSuiteAccount('rides-order');
    for (let index = 0; index < WRITTEN_RIDES; index += 1) rideEndedAgo(account, index * 10);

    const expected = publishedRideOrder(account);
    assert.equal(expected.length, WRITTEN_RIDES);

    const first = await ridesOf(account, `?limit=${PAGE_SIZE}`);
    assert.equal(first.status, 200, first.text);
    assert.deepEqual(identifiersOf(first.json), expected.slice(0, PAGE_SIZE));
    assert.match(first.json.next_cursor, CURSOR_ALPHABET, 'the first page offered no cursor');

    const second = await ridesOf(account, `?limit=${PAGE_SIZE}&cursor=${first.json.next_cursor}`);
    assert.equal(second.status, 200, second.text);
    assert.deepEqual(identifiersOf(second.json), expected.slice(PAGE_SIZE));
    assert.equal(second.json.next_cursor, null, 'the last page offered a continuation');
  });

  test('is empty for an account that has never ridden', async () => {
    const account = await newSuiteAccount('rides-empty');

    const answer = await ridesOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.deepEqual(answer.json.items, []);
    assert.equal(answer.json.next_cursor, null);
  });

  // A reservation given back, one that ran out and the rental in force are not rides: the history
  // answers what a person rode, and the rental in force is what the current read answers.
  test('leaves out the cancelled, the expired and the rental in force', async () => {
    const account = await newSuiteAccount('rides-stages');
    const { rentalId } = rideEndedAgo(account, 30);
    const [live] = await availableVehicles(1);
    insertRental({ account, stage: 'cancelled', at: `'${referenceMoment}'::timestamptz`, endedSecondsAgo: 20 });
    insertRental({ account, stage: 'expired', at: `'${referenceMoment}'::timestamptz`, endedSecondsAgo: 10 });
    insertRental({ account, stage: 'reserved', at: `'${referenceMoment}'::timestamptz`, vehicleId: live });

    const answer = await ridesOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.deepEqual(identifiersOf(answer.json), [rentalId]);
  });

  // The database admits one invoice per ride but does not require one, and the contract publishes the
  // invoice of every ride it lists: a history that silently lost a ride would be worse than a refusal.
  test('refuses to publish a finished ride no invoice was written for', async () => {
    const account = await newSuiteAccount('rides-uninvoiced');
    insertCompletedRide({ account, at: `'${referenceMoment}'::timestamptz`, endedSecondsAgo: 30, withInvoice: false });

    const answer = await ridesOf(account);
    assert.equal(answer.status, 500, answer.text);
    assert.equal(answer.json.code, 'INTERNAL_ERROR');
  });
});

describe('the collection of invoices', () => {
  test('publishes one page, newest first, with the state of each payment beside it', async () => {
    const account = await newSuiteAccount('invoices-page');
    const { rentalId, invoiceId } = rideEndedAgo(account, 30);

    const answer = await invoicesOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.deepEqual(invoiceIdentifiersOf(answer.json), [invoiceId]);

    const [view] = answer.json.items;
    assert.equal(view.invoice.rental_id, rentalId);
    assert.equal(view.invoice.total_amount_tyiyn, String(WRITTEN_TOTAL_TYIYN));
    assert.equal(view.invoice.lines.length, 2, 'an invoice publishes one line per mode');
    assert.equal(view.payment.status, 'pending');
    assert.match(view.invoice.issued_at, MOMENT);
  });

  test('walks the whole collection in the order the database holds it', async () => {
    const account = await newSuiteAccount('invoices-order');
    for (let index = 0; index < WRITTEN_RIDES; index += 1) rideEndedAgo(account, index * 10);

    const expected = publishedInvoiceOrder(account);
    assert.equal(expected.length, WRITTEN_RIDES);

    const first = await invoicesOf(account, `?limit=${PAGE_SIZE}`);
    assert.equal(first.status, 200, first.text);
    assert.deepEqual(invoiceIdentifiersOf(first.json), expected.slice(0, PAGE_SIZE));

    const second = await invoicesOf(account, `?limit=${PAGE_SIZE}&cursor=${first.json.next_cursor}`);
    assert.equal(second.status, 200, second.text);
    assert.deepEqual(invoiceIdentifiersOf(second.json), expected.slice(PAGE_SIZE));
    assert.equal(second.json.next_cursor, null, 'the last page offered a continuation');
  });

  test('is empty for an account that has never been charged', async () => {
    const account = await newSuiteAccount('invoices-empty');

    const answer = await invoicesOf(account);
    assert.equal(answer.status, 200, answer.text);
    assert.deepEqual(answer.json.items, []);
    assert.equal(answer.json.next_cursor, null);
  });

  // An invoice of another account and one that never existed are one answer, so the answer cannot be
  // used to learn that somebody else's invoice exists.
  test('does not publish an invoice of another account, and its read answers as absent', async () => {
    const owner = await newSuiteAccount('invoices-owner');
    const stranger = await newSuiteAccount('invoices-stranger');
    const { invoiceId } = rideEndedAgo(owner, 30);

    const collection = await invoicesOf(stranger);
    assert.equal(collection.status, 200, collection.text);
    assert.deepEqual(collection.json.items, []);

    const read = await invoiceOf(stranger, invoiceId);
    assert.equal(read.status, 404, read.text);
    assert.equal(read.json.code, 'RESOURCE_NOT_FOUND');
  });
});

describe('a cursor that belongs somewhere else', () => {
  /** Every refusal a cursor meets is the one the contract declares, so none of them can be told apart. */
  async function refuses(page) {
    const answer = await page;
    assert.equal(answer.status, 400, answer.text);
    assert.equal(answer.json.code, 'INVALID_CURSOR');
  }

  test('is refused by both collections when it is not a cursor at all', async () => {
    const account = await newSuiteAccount('cursor-malformed');

    await refuses(ridesOf(account, '?cursor=not-a-cursor'));
    await refuses(invoicesOf(account, '?cursor=not-a-cursor'));
  });

  test('is refused when its payload was edited after it was signed', async () => {
    const account = await newSuiteAccount('cursor-tampered');
    for (let index = 0; index < WRITTEN_RIDES; index += 1) rideEndedAgo(account, index * 10);

    const first = await ridesOf(account, '?limit=1');
    assert.match(first.json.next_cursor, CURSOR_ALPHABET, first.text);

    await refuses(ridesOf(account, `?limit=1&cursor=${tamper(first.json.next_cursor)}`));
  });

  test('is refused when it was issued for another account', async () => {
    const owner = await newSuiteAccount('cursor-owner');
    const stranger = await newSuiteAccount('cursor-stranger');
    for (let index = 0; index < WRITTEN_RIDES; index += 1) rideEndedAgo(owner, index * 10);

    const first = await ridesOf(owner, '?limit=1');
    const elsewhere = issueCursor(
      { operation: 'getRides', owner: ownerOf(owner), params: { limit: '1' } },
      positionOf(first.json.next_cursor),
    );

    await refuses(ridesOf(stranger, `?limit=1&cursor=${elsewhere}`));
  });

  // A cursor binds the parameters it was issued under, so a page asked for under another size is a
  // page of another collection: the two would otherwise disagree about where the next page starts.
  test('is refused when the page it continues was read under another size', async () => {
    const account = await newSuiteAccount('cursor-limit');
    for (let index = 0; index < WRITTEN_RIDES; index += 1) rideEndedAgo(account, index * 10);

    const first = await ridesOf(account, '?limit=1');
    assert.match(first.json.next_cursor, CURSOR_ALPHABET, first.text);

    await refuses(ridesOf(account, `?limit=2&cursor=${first.json.next_cursor}`));
  });

  test('is refused when it was issued for the other collection', async () => {
    const account = await newSuiteAccount('cursor-operation');
    for (let index = 0; index < WRITTEN_RIDES; index += 1) rideEndedAgo(account, index * 10);

    const rides = await ridesOf(account, '?limit=1');
    assert.match(rides.json.next_cursor, CURSOR_ALPHABET, rides.text);

    await refuses(invoicesOf(account, `?limit=1&cursor=${rides.json.next_cursor}`));
  });
});

describe('both collections without a session', () => {
  // The owner of a page is the session and never a parameter, so a request that carries no session
  // names nobody: it is refused rather than answered with somebody's history.
  test('are refused rather than answered for nobody', async () => {
    for (const path of [RIDES_PATH, INVOICES_PATH]) {
      const answer = await call(path);
      assert.equal(answer.status, 401, `${path}: ${answer.text}`);
      assert.equal(answer.json.code, 'AUTHENTICATION_REQUIRED');
    }
  });
});
