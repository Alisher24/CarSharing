// The one door into the application: what a person is told about a field before anything is sent, how
// they close the window without answering it, and what signing in from a link to their own history
// leaves them reading.
//
// These are the checks no HTTP suite can stand in for: the window is the browser's own dialog, the
// cursor and the Esc key are the browser's, and the address a sign-in survives is one the browser
// navigated to.
import { expect, test } from '@playwright/test';
import { sql } from '../../scripts/service.mjs';
import {
  SIGN_IN_ACTION,
  availableModel,
  book,
  email,
  endRidesOf,
  openCabinet,
  signIn,
  signOut,
  signUp,
} from './person.mjs';
import { restoreScenario } from './scenario.mjs';

/** The prefix every account of these checks is registered under, which is what the clean-up names. */
const ACCOUNT_PREFIX = 'entry';

/** The two operations the entry form sends, which a form that has something to complain about must not. */
const SEND_PATHS = ['/api/v1/auth/register', '/api/v1/auth/login'];

/** The addresses of the cabinet, which are the outward behaviour one check here is about. */
const INVOICES_ADDRESS = '/account/invoices';

/** What the window is, and what the tab that registers and its action are called. */
const WINDOW = '.entry-window';
const REGISTER_TAB = 'Регистрация';
const REGISTER_ACTION = 'Зарегистрироваться';

/** Why the rules refuse the two fields, in the words the interface fixes for them. */
const ADDRESS_MALFORMED = 'Неверный формат адреса: нужен вид name@example.com';
const PASSWORD_LENGTH = 'Пароль от 12 до 128 символов';

/** An address of the wrong shape and a password one code point short of the twelve the contract wants. */
const MALFORMED_ADDRESS = 'someone.example.test';
const SHORT_PASSWORD = 'correcthors';

/** The controls of a ride, in the words the interface fixes for them. */
const START_ACTION = 'Начать поездку';
const FINISH_ACTION = 'Завершить поездку';

/** What the panel says once the service confirmed that the ride is over. */
const RIDE_FINISHED = 'Поездка завершена';

/** How long a check waits for an invoice the service writes after the ride is over. */
const RECONCILIATION_PATIENCE_MS = 20_000;

test.beforeEach(() => {
  endRidesOf(ACCOUNT_PREFIX);
  restoreScenario();
  // Every check registers its own account, and the checks share one address. Clearing the counters
  // is the harness standing in for the passage of time, which is also how access returns.
  sql('DELETE FROM rate_limit_counters');
});

test.afterEach(() => {
  endRidesOf(ACCOUNT_PREFIX);
});

test('a field that cannot be sent is marked and explained rather than sent', async ({ page }) => {
  const sent = await watchSending(page);

  await page.goto('/');
  await headerControl(page).click();
  await expect(page.locator(WINDOW)).toBeVisible();

  // The window opens on the tab that signs somebody in, so somebody registering chooses theirs.
  await page.getByRole('button', { name: REGISTER_TAB }).click();
  await page.locator('#account-email-field').fill(MALFORMED_ADDRESS);
  await page.locator('#account-password-field').fill(SHORT_PASSWORD);
  await page.getByRole('button', { name: REGISTER_ACTION }).click();

  // The address is the first field that is wrong, so it is the one the cursor is put on, and each
  // field says what it is waiting for instead of being told apart by a colour alone.
  await expect(page.locator('#account-email-field')).toBeFocused();
  await expect(page.locator('#account-email-field')).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByTestId('email-error')).toHaveText(ADDRESS_MALFORMED);
  await expect(page.getByTestId('password-error')).toHaveText(PASSWORD_LENGTH);

  // A field that has been checked is checked while it is typed into, so a corrected address stops
  // being marked without the form being sent again.
  await page.locator('#account-email-field').fill(email(ACCOUNT_PREFIX));
  await expect(page.getByTestId('email-error')).toHaveCount(0);
  await expect(page.getByTestId('password-error')).toBeVisible();

  // None of that reached the server: the rules the contract states are known here, and the two
  // operations that could have been asked are the ones that were not.
  expect(sent).toEqual([]);
});

test('Esc closes the window and gives the focus back to the control that opened it', async ({ page }) => {
  await page.goto('/');
  const control = headerControl(page);

  await control.click();
  await expect(page.locator(WINDOW)).toBeVisible();

  await page.keyboard.press('Escape');

  // Closing leaves the person where they were rather than at the top of the page: the control that
  // opened the window has the focus again.
  await expect(page.locator(WINDOW)).toBeHidden();
  await expect(control).toBeFocused();
});

test('signing in from the cabinet at one invoice keeps that invoice', async ({ page }) => {
  const address = email(ACCOUNT_PREFIX);

  await page.goto('/');
  await signUp(page, address);
  await ride(page, await availableModel());

  // The ride links to the invoice it was charged by, which is an address a person can send on.
  await openCabinet(page, address);
  await page.locator('.feed-row-link').first().click();
  await expect(page.locator('.invoice-card-total')).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });
  const invoiceAddress = new URL(page.url()).pathname;
  expect(invoiceAddress.startsWith(INVOICES_ADDRESS)).toBe(true);

  await signOut(page, address);

  // The link is opened by somebody who is not signed in, so the window is shown over the cabinet and
  // the address is not given up: signing in reads the invoice it names rather than the map.
  await page.goto(invoiceAddress);
  await expect(page.locator(WINDOW)).toBeVisible();
  await expect(page.locator('#account-email-field')).toBeVisible();

  await signIn(page, address);

  await expect(page).toHaveURL(new RegExp(`${invoiceAddress}$`));
  await expect(page.locator('.invoice-card-total')).toBeVisible();
});

/** The one account control of the header, which is what opens the window from any screen. */
function headerControl(page) {
  return page.getByRole('banner').getByRole('button', { name: SIGN_IN_ACTION });
}

/**
 * Watches the two operations the entry form sends, and answers what was sent to them. A form that
 * has something to complain about must send neither, so what this records stays empty.
 */
async function watchSending(page) {
  const sent = [];

  await page.route('**/api/v1/auth/*', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (SEND_PATHS.includes(path)) sent.push(path);
    await route.continue();
  });

  return sent;
}

/** Books one vehicle, rides it and ends the ride, which is what leaves an invoice to be read. */
async function ride(page, model) {
  await book(page, model);
  await page.getByRole('button', { name: START_ACTION }).click();
  await page.getByRole('button', { name: FINISH_ACTION }).first().click();
  // The question is asked before the ending is sent, because an ending is not undone by asking again.
  await page.locator('.reservation-panel-confirm').getByRole('button', { name: FINISH_ACTION }).click();
  await expect(page.locator('.reservation-panel-time')).toHaveText(RIDE_FINISHED);
}
