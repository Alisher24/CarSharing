// What a person does with their own history in a real browser: rides, reads the ride back in the
// cabinet, opens the invoice it links to, reloads that address and is still on it.
//
// These are the checks no HTTP suite can stand in for: the addresses are the browser's, a reload is
// the browser's, and losing the network is the browser's too. The server's own half of the same
// acceptance lines is proved by the suites in scripts/acceptance.
import { expect, test } from '@playwright/test';
import { sql } from '../../scripts/service.mjs';
import { availableModel, book, CABINET_ACTION, email, SIGN_IN_ACTION, register, signUp } from './person.mjs';
import { restoreScenario } from './scenario.mjs';

/** The addresses of the cabinet, which are the outward behaviour these checks are about. */
const RIDES_ADDRESS = '/account/rides';
const INVOICES_ADDRESS = '/account/invoices';

/** The address that switches the periodic reconciliation off, which only a test asks for. */
const WITHOUT_RECONCILIATION = '/?reconcile=off';

/** Where the private stream of the signed-in person lives, which one check breaks. */
const PRIVATE_EVENTS = '**/api/v1/me/events';

/** The controls of a ride, in the words the interface fixes for them. */
const START_ACTION = 'Начать поездку';
const FINISH_ACTION = 'Завершить поездку';

/** What the panel says once the service confirmed that the ride is over. */
const RIDE_FINISHED = 'Поездка завершена';

/** What the header of the cabinet says when the account has ridden nothing. */
const NO_RIDES = 'Вы ещё не совершали поездок';

/** How long a check waits for a change that only the reconciliation can bring. */
const RECONCILIATION_PATIENCE_MS = 20_000;

test.beforeEach(() => {
  endPreviousRides();
  restoreScenario();
  // Every check registers its own account, and the checks share one address. Clearing the counters
  // is the harness standing in for the passage of time, which is also how access returns.
  sql('DELETE FROM rate_limit_counters');
});

test.afterEach(() => {
  endPreviousRides();
});

/** Removes what the accounts of these checks hold, so the prepared demonstration can be put back. */
function endPreviousRides() {
  const mine = "(SELECT id FROM users WHERE email LIKE 'cabinet-%@example.test')";
  sql(`DELETE FROM outbox WHERE recipient_id IN ${mine}`);
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
}

test('a finished ride is read back in the cabinet, and its invoice has an address of its own', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto('/');
    await signUp(page, email('cabinet'));
    await ride(page, await availableModel());

    // The cabinet is reached from the header, and the address it leads to is the feed of rides.
    await page.getByRole('link', { name: CABINET_ACTION }).click();
    await expect(page).toHaveURL(new RegExp(`${RIDES_ADDRESS}$`));
    await expect(page.locator('.feed-row')).toHaveCount(1);

    // The ride links to the invoice it was charged by, which is an address a person can send on.
    await page.locator('.feed-row-link').first().click();
    await expect(page).toHaveURL(new RegExp(`${INVOICES_ADDRESS}/[0-9a-f-]+$`));
    const invoiceAddress = new URL(page.url()).pathname;
    await expect(page.locator('.invoice-card-total')).toBeVisible();
    await expect(page.locator('.invoice-line')).toHaveCount(2, { timeout: RECONCILIATION_PATIENCE_MS });

    // A reload of that address is the browser's half of recovering after a restart: what is on
    // screen is read from the service again rather than remembered by this tab.
    await page.reload();
    await expect(page).toHaveURL(new RegExp(`${invoiceAddress}$`));
    await expect(page.locator('.invoice-card-total')).toBeVisible();

    // The tab of the feed the invoice belongs to stays the current one while it is being read.
    await expect(page.locator('.cabinet-tab[aria-current="page"]')).toHaveText('Счета');
  } finally {
    await context.close();
  }
});

test('a cabinet address opened without a session keeps the address the person was going to', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    // Nobody is signed in, so the cabinet shows the entry window over the feed rather than sending a
    // person back to the map and losing where they were going. The header still offers the way in.
    await page.goto(RIDES_ADDRESS);
    await expect(page.locator('.cabinet')).toBeVisible();
    await expect(page.locator('#account-email-field')).toBeVisible();
    await expect(page.getByRole('banner').getByRole('button', { name: SIGN_IN_ACTION })).toBeVisible();

    await register(page, email('cabinet'));

    // Signing in leaves the person where they were going, with the feed that address names.
    await expect(page).toHaveURL(new RegExp(`${RIDES_ADDRESS}$`));
    await expect(page.locator('.resource-notice')).toHaveText(new RegExp(NO_RIDES));
  } finally {
    await context.close();
  }
});

test('the cabinet is read again after the private stream breaks, and survives losing the network', async ({
  browser,
}) => {
  const context = await browser.newContext();

  // The subscription is answered once and then refused, which is what a client sees when its stream
  // breaks: the cabinet must then be brought up to date by reading rather than by a signal.
  let answered = false;
  await context.route(PRIVATE_EVENTS, async (route) => {
    if (answered) {
      await route.abort();
      return;
    }
    answered = true;
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'text/event-stream' },
      body: `retry: 3000\nevent: ready\ndata: {"server_time":"${new Date().toISOString()}"}\n\n`,
    });
  });

  const page = await context.newPage();
  try {
    await page.goto(WITHOUT_RECONCILIATION);
    await signUp(page, email('cabinet'));
    await ride(page, await availableModel());

    await page.goto(RIDES_ADDRESS);
    await expect(page.locator('.feed-row')).toHaveCount(1, { timeout: RECONCILIATION_PATIENCE_MS });

    // Losing the network entirely breaks the stream and the reads alike, which is the browser's own
    // offline rather than a transport that is merely behind. What was read stays on screen, and the
    // application says the link is gone instead of showing an empty history.
    await context.setOffline(true);
    await expect(page.locator('.connection-indicator-stale, .connection-indicator-failed')).toBeVisible({
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(page.locator('.feed-row')).toHaveCount(1);

    await context.setOffline(false);
    await expect(page.locator('.connection-indicator-ready')).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });
    await expect(page.locator('.feed-row')).toHaveCount(1);
  } finally {
    await context.setOffline(false);
    await context.close();
  }
});

/** Books one vehicle, rides it and ends the ride, which is what leaves a history to read. */
async function ride(page, model) {
  await book(page, model);
  await page.getByRole('button', { name: START_ACTION }).click();
  await page.getByRole('button', { name: FINISH_ACTION }).first().click();
  // The question is asked before the ending is sent, because an ending is not undone by asking again.
  await page.locator('.reservation-panel-confirm').getByRole('button', { name: FINISH_ACTION }).click();
  await expect(page.locator('.reservation-panel-time')).toHaveText(RIDE_FINISHED);
}
