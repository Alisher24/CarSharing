// What a person does with a reservation in a real browser: two independent contexts see one vehicle
// taken and given back, the panel above the map survives everything that may be closed around it,
// and the day's allowance is stated rather than guessed at.
//
// The periodic reconciliation is switched off where a check measures delivery, so what is measured is
// the signal and not a poll that would have found the change anyway. The shipped interval is left
// alone everywhere else.
import { expect, test } from '@playwright/test';
import { sql } from '../../scripts/service.mjs';
import {
  availableModel,
  BOOK_ACTION,
  book,
  CABINET_ACTION,
  CANCEL_ACTION,
  CONFIRM_ACTION,
  email,
  openVehicle,
  signUp,
  statusOf,
} from './person.mjs';
import { restoreScenario } from './scenario.mjs';

/** The bound a committed change must reach a connected client within. */
const DELIVERY_BOUND_MS = 2000;

/** The address that switches the periodic reconciliation off, which only a test asks for. */
const WITHOUT_RECONCILIATION = '/?reconcile=off';

/** How long a change may take to appear through reconciliation alone. */
const RECONCILIATION_PATIENCE_MS = 20_000;

const LIMIT_SPENT = 'Бесплатная бронь использована';
const NOTHING_CURRENT = 'Текущей брони нет';

test.beforeEach(() => {
  endPreviousReservations();
  restoreScenario();
  // Every check registers its own account, and the checks share one address. Clearing the counters
  // is the harness standing in for the passage of time, which is also how access returns in
  // production.
  sql('DELETE FROM rate_limit_counters');
});

// A check ends the reservation it made: the demonstration refuses to be put back while a rental of a
// person's stands on one of its vehicles, and the checks after it start from the prepared scenario.
test.afterEach(() => {
  endPreviousReservations();
});

/** Removes what the accounts of these checks hold, so the prepared demonstration can be put back. */
function endPreviousReservations() {
  const mine = "(SELECT id FROM users WHERE email LIKE 'reservation-%@example.test')";
  sql(`DELETE FROM outbox WHERE resource_id IN (SELECT id FROM rentals WHERE user_id IN ${mine})`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
}

test('one client books a vehicle and the other sees it taken and free again', async ({ browser }) => {
  const model = await availableModel();

  const booking = await browser.newContext();
  const watching = await browser.newContext();
  const person = await booking.newPage();
  const visitor = await watching.newPage();

  try {
    await person.goto('/');
    await visitor.goto(WITHOUT_RECONCILIATION);

    await signUp(person, email('reservation'));
    await expect(statusOf(visitor, model)).toHaveAttribute('data-status', 'available');

    await openVehicle(person, model);
    // The card prices the vehicle through the same rates the panel shows, so what a person agrees to
    // is what the service published and one rental is not priced two ways on one screen.
    await expect(person.locator('.vehicle-card .tariff-rates')).toContainText('сома');
    await person.getByRole('button', { name: BOOK_ACTION }).click();
    // The conditions are read before anything is sent, and the panel appears only after the server
    // answered: what a person sees is the reservation the service holds.
    await person.getByRole('button', { name: CONFIRM_ACTION }).click();
    await expect(person.locator('.reservation-panel-time')).toContainText('Осталось', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });

    // The panel of the person who booked is read back from the server, so the reservation is
    // committed; the other client runs no reconciliation, so what reaches it within the bound is the
    // delivered signal rather than a poll that would have found the change anyway.
    const committedAt = Date.now();
    await expect(statusOf(visitor, model)).toHaveAttribute('data-status', 'reserved', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    const arrivedIn = Date.now() - committedAt;
    expect(arrivedIn, `the change reached the second client ${arrivedIn} ms after the commit`).toBeLessThan(
      DELIVERY_BOUND_MS,
    );

    // The rates the panel shows are the ones the reservation stores, and they are on the panel that
    // stays above the map. The card shows its own rates while it is open, so the panel is named.
    await expect(person.locator('.reservation-panel .tariff-rates')).toContainText('сома');

    await person.getByRole('button', { name: CANCEL_ACTION }).click();
    await person.getByRole('button', { name: CANCEL_ACTION }).last().click();
    await expect(person.locator('.reservation-panel-empty')).toHaveText(NOTHING_CURRENT, {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(statusOf(visitor, model)).toHaveAttribute('data-status', 'available', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
  } finally {
    await booking.close();
    await watching.close();
  }
});

test('the panel comes back after a reload and outlives every panel around it', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const model = await availableModel();

  try {
    await page.goto('/');
    await signUp(page, email('reservation'));
    await book(page, model);
    await expect(page.locator('.reservation-panel-time')).toContainText('Осталось');

    // A reload restores the reservation through the current read, not through anything the browser
    // remembered about the command.
    await page.reload();
    await expect(page.locator('.reservation-panel-time')).toContainText('Осталось', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });

    // Closing the card and walking to the cabinet and back are not reasons to lose sight of a
    // reservation that is running: the reservation lives above every screen, and the map shows the
    // same one when a person returns to it.
    await openVehicle(page, model);
    await page.locator('.vehicle-card-close').click();
    await expect(page.locator('.vehicle-card')).toHaveCount(0);
    await page.getByRole('link', { name: CABINET_ACTION }).click();
    await expect(page.locator('.cabinet')).toBeVisible();
    await page.goBack();
    await expect(page.locator('.reservation-panel-time')).toContainText('Осталось');
  } finally {
    await context.close();
  }
});

test('the spent allowance is stated and stops the booking control', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto('/');
    await signUp(page, email('reservation'));
    await book(page, await availableModel());
    await expect(page.locator('.reservation-panel-time')).toContainText('Осталось');

    // The allowance is published whether or not a rental is current, and the next free reservation
    // is named in the timezone the service keeps its days in.
    await expect(page.locator('.reservation-panel-limit')).toContainText(LIMIT_SPENT);

    // Another free vehicle is still offered, and its control states the spent day rather than
    // pretending that booking is possible.
    await page.locator('.vehicle-card-close').click();
    await openVehicle(page, await availableModel());
    await expect(page.locator('.vehicle-card-booking button').first()).toBeDisabled();
    await expect(page.locator('.vehicle-card-limit')).toContainText(LIMIT_SPENT);
  } finally {
    await context.close();
  }
});
