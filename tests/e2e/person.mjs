// What a person does in a browser check: registering, finding a vehicle, booking it and signing out
// again. The steps live here rather than in each suite because two suites drive the same interface,
// and a control renamed in one place must not leave the other asserting a screen that no longer
// exists.
import { expect } from '@playwright/test';
import { SERVICE_ORIGIN, sql } from '../../scripts/service.mjs';

/** The password every account of these checks is registered with. */
const PASSWORD = 'correcthorsebattery';

/** How long a change may take to appear through reconciliation alone: several times the delivery bound. */
export const RECONCILIATION_PATIENCE_MS = 20_000;

/** How often a check that waits for the stack asks again, which is what `until` polls with. */
const POLL_MS = 200;

/** What the booking control of a free vehicle says, before anything was asked. */
export const BOOK_ACTION = 'Забронировать на 15 минут';

/** What the confirmation of a booking says, which is the step that sends it. */
export const CONFIRM_ACTION = 'Использовать бесплатную бронь';

/** What the panel offers while a reservation is running. */
export const CANCEL_ACTION = 'Отменить бронь';

/** The one account control of the header, in each of the two states it has. */
export const SIGN_IN_ACTION = 'Вход';
export const CABINET_ACTION = 'Кабинет';

/** What a fresh address of one check looks like, so no check depends on another one's leavings. */
export function email(prefix) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@example.test`;
}

/** One vehicle the service publishes as free to take, by the model it displays. */
export async function availableModel() {
  return (await availableVehicle()).model;
}

/**
 * One vehicle the service publishes as free to take, whole. A check that prepares the vehicle through
 * the demonstration control addresses it by identifier, and the identifier the catalog publishes is
 * the one that control takes.
 */
export async function availableVehicle() {
  const answer = await fetch(`${SERVICE_ORIGIN}/api/v1/vehicles`).then((response) => response.json());
  const [free] = answer.items.filter((vehicle) => vehicle.status === 'available');
  if (free === undefined) throw new Error('the demonstration published no free vehicle');

  return free;
}

/**
 * The free vehicle of one powertrain, which a check that spends a named source asks for: an electric
 * vehicle is moved by its battery alone, so draining that one reserve is what ends its ride.
 */
export async function availableVehicleOfPowertrain(powertrainType) {
  const answer = await fetch(`${SERVICE_ORIGIN}/api/v1/vehicles`).then((response) => response.json());
  const found = answer.items.find(
    (vehicle) => vehicle.status === 'available' && vehicle.powertrain_type === powertrainType,
  );
  if (found === undefined) throw new Error(`the demonstration published no free ${powertrainType} vehicle`);

  return found;
}

/** The row one model stands in, which is where the list states what a vehicle is doing. */
function rowOf(page, model) {
  return page.locator('.fleet-row', { has: page.locator('.fleet-row-model', { hasText: model }) });
}

/** The state one row publishes. */
export function statusOf(page, model) {
  return rowOf(page, model).locator('.fleet-row-status');
}

/** Opens one vehicle's card from the list, which is how a person reaches the booking control. */
export async function openVehicle(page, model) {
  await rowOf(page, model).click();
  await expect(page.locator('.vehicle-card-model')).toHaveText(model);
}

/** Books one free vehicle through the confirmation the interface asks for. */
export async function book(page, model) {
  await openVehicle(page, model);
  await page.getByRole('button', { name: BOOK_ACTION }).click();
  await page.getByRole('button', { name: CONFIRM_ACTION }).click();
}

/**
 * Registers a fresh account through the entry panel above the map, as a person would. The panel
 * closes itself: the one control of the header states what a person can do next, so a session turns
 * it into the way into the cabinet.
 */
export async function signUp(page, address) {
  await page.getByRole('button', { name: SIGN_IN_ACTION }).click();
  await register(page, address);
  await expect(page.getByRole('link', { name: CABINET_ACTION })).toBeVisible();
}

/** Fills whichever entry form is on screen and registers, which is what a person does to get in. */
export async function register(page, address) {
  await page.locator('#account-email-field').fill(address);
  await page.locator('#account-password-field').fill(PASSWORD);
  await page.getByRole('button', { name: 'Зарегистрироваться' }).click();
}

/** Opens the cabinet, which is where the account and the way out of it live. */
export async function openCabinet(page, address) {
  await page.getByRole('link', { name: CABINET_ACTION }).click();
  await expect(page.locator('[data-testid="account-email"]')).toHaveText(address);
}

/** Signs the person on screen out, which the cabinet is the one place to do from. */
export async function signOut(page, address) {
  await openCabinet(page, address);
  await page.getByRole('button', { name: 'Выйти' }).click();
  await expect(page.locator('[data-testid="account-email"]')).toHaveCount(0);
}

/**
 * Waits until the panel above the map says what a check expects of the command it sent, and answers
 * what it said.
 *
 * The text is read from the whole panel and polled, rather than asserted on the notice element: the
 * notice is replaced by the next render — every answer to a command reads the state again — so an
 * assertion that looks for the element at one moment can miss a message that really was shown. The
 * panel's text keeps what it said, so a message that appeared is still read here.
 *
 * A panel that never says it fails with the whole text it did show, because what a panel says instead
 * is the first thing a reader of the failure needs.
 */
export async function panelSays(page, expected, patienceMs = RECONCILIATION_PATIENCE_MS) {
  let said = '';
  try {
    await expect
      .poll(
        async () => {
          said = await page.locator(PANEL).innerText();
          return said.includes(expected);
        },
        { timeout: patienceMs },
      )
      .toBe(true);
  } catch {
    throw new Error(`the panel never said ${JSON.stringify(expected)}; it said:\n${said}`);
  }

  return said;
}

/** The panel above the map, which every screen showing a rental carries. */
export const PANEL = '.reservation-panel';

/**
 * Removes what the accounts of one prefix hold, so the prepared demonstration can be put back: the
 * restoration refuses while a rental of a person's stands on one of its vehicles. Every suite names
 * its own prefix, so this is one statement of the tables a suite owes rather than one copy per suite.
 */
export function endRidesOf(prefix) {
  const mine = `(SELECT id FROM users WHERE email LIKE '${prefix}-%')`;
  sql(`DELETE FROM outbox WHERE recipient_id IN ${mine}`);
  sql(`DELETE FROM notifications WHERE user_id IN ${mine}`);
  sql(`DELETE FROM invoices WHERE user_id IN ${mine}`);
  sql(`DELETE FROM idempotency_requests WHERE user_id IN ${mine}`);
  sql(`DELETE FROM rentals WHERE user_id IN ${mine}`);
}

/**
 * Waits until a condition the stack reaches on its own holds, or fails with what it waited for. The
 * condition answers with what it observed, and its answer is awaited before it is judged: a condition
 * that reads a service is asynchronous, and asking it twice at once would be two reads.
 */
export async function until(reached, complaint, patienceMs = RECONCILIATION_PATIENCE_MS) {
  const deadline = Date.now() + patienceMs;
  for (;;) {
    const value = await reached();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`${complaint} within ${patienceMs} ms`);
    await new Promise((resolve) => setTimeout(resolve, POLL_MS));
  }
}
