// What a person whose reservation is running out sees in a real browser against the assembled stack:
// the one warning of the last minute, the action that marks it read on the server, what a reload
// shows, and what is left of it once the reservation is over.
//
// The deadline of a real reservation is moved into its last minute with SQL, because no check can
// wait fifteen minutes. Everything after that is the service's own path: the worker's sweep creates
// the warning and its personal signal, and the client reads the collection the signal points at.
//
// Where a check measures delivery, the periodic reconciliation is switched off by the address the
// page loads, so what is measured is the delivered signal rather than a poll that would have found
// the change anyway. It is left on where the check is about reading as the repair.
import assert from 'node:assert/strict';
import { expect, test } from '@playwright/test';
import { closeStream, waitForFrame, watchEventStream } from '../../scripts/acceptance/events.mjs';
import { moveDeadline } from '../../scripts/acceptance/reservations.mjs';
import { compose, sql } from '../../scripts/service.mjs';
import { availableModel, book, CANCEL_ACTION, email, register, signOut, signUp } from './person.mjs';
import { restoreScenario } from './scenario.mjs';

/** The bound a committed change must reach a connected client within. */
const DELIVERY_BOUND_MS = 2000;

/** The address that switches the periodic reconciliation off, which only a test asks for. */
const WITHOUT_RECONCILIATION = '/?reconcile=off';

/** How long a change may take to appear through reconciliation alone. */
const RECONCILIATION_PATIENCE_MS = 20_000;

/** The one action the warning offers, in the words the interface fixes. */
const READ_ACTION = 'Прочитано';

/** What the panel says when nothing is current. */
const NOTHING_CURRENT = 'Текущей брони нет';

/** The public stream, which is the one connection no account owns. */
const PUBLIC_EVENTS_PATH = '/api/v1/events';

/**
 * How much of its last minute a prepared reservation is given. It is inside the minute the warning
 * belongs to, and it leaves room for the three reloads a check of the read state performs.
 */
const LAST_MINUTE_SECONDS = 55;

/** Where the checks of this suite read the interface. */
const WARNING = '.reservation-warning';

test.beforeEach(() => {
  endPreviousWarnings();
  restoreScenario();
  // Every check registers its own account, and the checks share one address. Clearing the counters is
  // the harness standing in for the passage of time, which is also how access returns in production.
  sql('DELETE FROM rate_limit_counters');
});

// A check ends the reservation it made: the demonstration refuses to be put back while a rental of a
// person's stands on one of its vehicles, and the checks after it start from the prepared scenario.
test.afterEach(() => {
  endPreviousWarnings();
});

test('the warning arrives on the signal within two seconds of the commit', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto(WITHOUT_RECONCILIATION);
    const { model, rentalId } = await prepareReservation(page, LAST_MINUTE_SECONDS);

    // The warning is created by the worker's own sweep and reaches the page through its personal
    // signal; the commit it is measured from is the one the database wrote. The measurement is
    // reported as well as asserted, so a run says what the delivery actually cost.
    await expect(page.locator(WARNING)).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });
    const arrivedIn = Date.now() - committedAt(rentalId);
    console.log(`the warning reached the browser ${arrivedIn} ms after the commit`);
    expect(arrivedIn, `the warning reached the browser ${arrivedIn} ms after the commit`).toBeLessThan(
      DELIVERY_BOUND_MS,
    );

    // What the warning says: which vehicle is held, when the reservation ends and what is left.
    await expect(page.locator('.reservation-warning-vehicle')).toHaveText(model);
    await expect(page.locator('.reservation-warning-deadline')).toContainText('Бронь закончится');
    await expect(page.locator('.reservation-warning-left')).toContainText(/Осталось 0:\d\d/);
    await expect(page.getByRole('button', { name: READ_ACTION })).toBeVisible();

    // The panel beside it keeps its own countdown and its cancellation, and the reservation holds one
    // warning rather than one per read.
    await expect(page.locator('.reservation-panel-time')).toContainText('Осталось');
    await expect(page.getByRole('button', { name: CANCEL_ACTION })).toBeVisible();
    expect(warningsOf(rentalId)).toBe(1);
  } finally {
    await context.close();
  }
});

test('a reload before reading shows the same single warning, and reading keeps it read', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto(WITHOUT_RECONCILIATION);
    const { rentalId } = await prepareReservation(page, LAST_MINUTE_SECONDS);
    await expect(page.locator(WARNING)).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });
    const written = storedWarning(rentalId);

    // A reload remembers nothing: the warning comes back from the collection, and the server wrote no
    // second notification for the same reservation.
    await page.reload();
    await expect(page.locator(WARNING)).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });
    expect(warningsOf(rentalId)).toBe(1);
    assert.equal(storedWarning(rentalId)?.id, written?.id, 'the reload showed another notification');

    await page.getByRole('button', { name: READ_ACTION }).click();
    await expect(page.locator(WARNING)).toHaveCount(0);

    // Read state is the server's: the version moved once, the moment was stored, and a reload does not
    // show the warning as new.
    const read = storedWarning(rentalId);
    assert.equal(read?.version, 2, 'the read did not move the version exactly once');
    assert.notEqual(read?.readAt ?? '', '', 'the read moment was not stored');

    await page.reload();
    await expect(page.locator('.reservation-panel-time')).toContainText('Осталось', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(page.locator(WARNING)).toHaveCount(0);
  } finally {
    await context.close();
  }
});

test('a warning the stream never delivers is repaired by reading the collection', async ({ browser }) => {
  const context = await browser.newContext();
  // Every attempt at the private subscription is refused, so nothing this check prepares arrives as a
  // signal: the periodic read of the collection is the only thing left that can show the warning.
  let refused = 0;
  await context.route('**/api/v1/me/events', (route) => {
    refused += 1;
    return route.abort();
  });

  const page = await context.newPage();
  try {
    await page.goto('/');
    const { rentalId } = await prepareReservation(page, LAST_MINUTE_SECONDS);

    await expect(page.locator(WARNING)).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });
    expect(warningsOf(rentalId)).toBe(1);
    expect(refused, 'the private subscription was established after all').toBeGreaterThan(0);
  } finally {
    await context.close();
  }
});

test('the warning stops being current when the reservation ends', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto(WITHOUT_RECONCILIATION);
    const { rentalId } = await prepareReservation(page, LAST_MINUTE_SECONDS);
    await expect(page.locator(WARNING)).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });

    // The deadline passes while the worker is running: the reservation is released and its warning is
    // deactivated in the transaction that releases it, and the client is told so by reading.
    moveDeadline(rentalId, -1);
    await expect(page.locator(WARNING)).toHaveCount(0, { timeout: RECONCILIATION_PATIENCE_MS });

    const stored = storedWarning(rentalId);
    assert.equal(stored?.active, false, 'the warning stayed current after the reservation ended');
    assert.equal(stored?.version, 2, 'the deactivation did not move the version exactly once');
    assert.equal(stageOf(rentalId), 'expired');
  } finally {
    await context.close();
  }
});

test('cancelling the reservation takes the warning with it', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto(WITHOUT_RECONCILIATION);
    const { rentalId } = await prepareReservation(page, LAST_MINUTE_SECONDS);
    await expect(page.locator(WARNING)).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });

    // The cancellation is the panel's own control, which the warning must not take away.
    await page.getByRole('button', { name: CANCEL_ACTION }).click();
    await page.getByRole('button', { name: CANCEL_ACTION }).last().click();

    await expect(page.locator(WARNING)).toHaveCount(0, { timeout: RECONCILIATION_PATIENCE_MS });
    await expect(page.locator('.reservation-panel-empty')).toHaveText(NOTHING_CURRENT, {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    assert.equal(storedWarning(rentalId)?.active, false, 'the warning of a cancelled reservation stayed current');
  } finally {
    await context.close();
  }
});

test('a deadline that passed while the worker was stopped produces no warning', async ({ browser }) => {
  compose('stop', 'worker');
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto('/');
    const { rentalId } = await prepareReservation(page, -1);

    // The person returns to a reservation that is already over: the reads fix the expiry, and a
    // warning about a reservation that ended is not one of the things they create.
    await page.reload();
    await expect(page.locator('.reservation-panel-empty')).toHaveText(NOTHING_CURRENT, {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(page.locator(WARNING)).toHaveCount(0);
    expect(warningsOf(rentalId)).toBe(0);
    assert.equal(stageOf(rentalId), 'expired', 'the read did not release the reservation');
  } finally {
    await context.close();
    compose('start', 'worker');
  }
});

test('signing out leaves no warning of the account that read it', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto(WITHOUT_RECONCILIATION);
    await prepareReservation(page, LAST_MINUTE_SECONDS);
    await expect(page.locator(WARNING)).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });

    await signOut(page);
    await expect(page.locator(WARNING)).toHaveCount(0);

    // Nobody signed in means nothing private is read, and the person who registers next reads their
    // own empty collection rather than the warning of the account before them.
    await register(page, email('warning-second'));
    await expect(page.locator(WARNING)).toHaveCount(0);
  } finally {
    await context.close();
  }
});

test('a personal warning never arrives on the public stream', async ({ browser }) => {
  const publicStream = await watchEventStream(PUBLIC_EVENTS_PATH);
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto(WITHOUT_RECONCILIATION);
    const { rentalId } = await prepareReservation(page, LAST_MINUTE_SECONDS);
    await waitForFrame(publicStream, (frame) => frame.event === 'ready');

    // The warning reaching the page is what makes the absence below meaningful: the signal of that
    // commit exists, and it is delivered to the account it belongs to rather than to every reader.
    await expect(page.locator(WARNING)).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });
    expect(warningsOf(rentalId)).toBe(1);
    assert.equal(
      publicStream.frames.some((frame) => frame.event === 'notification.changed'),
      false,
      'the public stream carried a personal notification',
    );
  } finally {
    closeStream(publicStream);
    await context.close();
  }
});

/**
 * One person with a reservation whose deadline has been moved by a stated number of seconds, and the
 * identifiers a check needs to read what the database holds. The reservation itself is made through
 * the interface, so the warning is about the reservation the service actually created.
 */
async function prepareReservation(page, deadlineSeconds) {
  const address = email('warning');
  const model = await availableModel();
  await signUp(page, address);
  await book(page, model);
  await expect(page.locator('.reservation-panel-time')).toContainText('Осталось');

  const rentalId = rentalOf(address);
  moveDeadline(rentalId, deadlineSeconds);
  return { address, model, rentalId };
}

/** The rental the person registered at one address holds, which is the one being prepared. */
function rentalOf(address) {
  return sql(
    `SELECT id FROM rentals
     WHERE user_id = (SELECT id FROM users WHERE email = '${address}') AND ended_at IS NULL`,
  );
}

/** What the database holds for the warning of one rental: its identity, version, activity and read moment. */
function storedWarning(rentalId) {
  const row = sql(
    `SELECT id || '|' || version || '|' || active || '|' || coalesce(read_at::text, '')
     FROM notifications WHERE rental_id = '${rentalId}'`,
  );
  if (row === '') return undefined;

  const [id, version, active, readAt] = row.split('|');
  return { id, version: Number(version), active: active === 'true', readAt };
}

/** How many warnings the database holds for one rental, which a reload must not double. */
function warningsOf(rentalId) {
  return Number(sql(`SELECT count(*) FROM notifications WHERE rental_id = '${rentalId}'`));
}

/** When the database committed the warning of one rental, which is what delivery is measured from. */
function committedAt(rentalId) {
  return Number(
    sql(`SELECT (extract(epoch FROM created_at) * 1000)::bigint FROM notifications WHERE rental_id = '${rentalId}'`),
  );
}

/** The stage the reservation of one rental stands in, which its warning follows. */
function stageOf(rentalId) {
  return sql(`SELECT stage FROM rentals WHERE id = '${rentalId}'`);
}

/** Removes what the accounts of these checks hold, so the prepared demonstration can be put back. */
function endPreviousWarnings() {
  const mine = "(SELECT id FROM users WHERE email LIKE 'warning-%@example.test')";
  sql(
    `DELETE FROM outbox
     WHERE recipient_id IN ${mine}
        OR resource_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})
        OR resource_id IN (SELECT id FROM notifications WHERE user_id IN ${mine})`,
  );
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
}
