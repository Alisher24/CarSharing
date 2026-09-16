// What a person does in a browser check: registering, finding a vehicle, booking it and signing out
// again. The steps live here rather than in each suite because two suites drive the same interface,
// and a control renamed in one place must not leave the other asserting a screen that no longer
// exists.
import { expect } from '@playwright/test';
import { SERVICE_ORIGIN } from '../../scripts/service.mjs';

/** The password every account of these checks is registered with. */
const PASSWORD = 'correcthorsebattery';

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
  const answer = await fetch(`${SERVICE_ORIGIN}/api/v1/vehicles`).then((response) => response.json());
  const [free] = answer.items.filter((vehicle) => vehicle.status === 'available');
  if (free === undefined) throw new Error('the demonstration published no free vehicle');

  return free.model;
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
