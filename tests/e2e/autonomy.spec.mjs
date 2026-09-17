// What the mandatory journey costs the network: the map and every screen a demonstration needs work
// without a single request beyond the local address, and the browser makes none.
//
// Localhost-only is the claim a person cannot check by looking at the screen, which is why it is
// checked here. The public address may also be reached on the loopback address of the machine, and
// every other request is refused rather than merely observed: a page that quietly waited for a tile
// server would then hang instead of passing on a slow one, so the check fails on what the page asked
// for rather than on how long it took.
import assert from 'node:assert/strict';
import { expect, test } from '@playwright/test';
import { MAILBOX_ORIGIN, SERVICE_ORIGIN } from '../../scripts/service.mjs';
import {
  availableModel,
  book,
  CABINET_ACTION,
  CANCEL_ACTION,
  email,
  endRidesOf,
  RECONCILIATION_PATIENCE_MS,
  signUp,
} from './person.mjs';
import { restoreScenario } from './scenario.mjs';

/** The prefix of every account these checks register, which is how their rows are found again. */
const ACCOUNT_PREFIX = 'offline';

/** The addresses this build serves. Anything else is a request the installation does not provide. */
const ALLOWED_ORIGINS = new Set([new URL(SERVICE_ORIGIN).origin, new URL(MAILBOX_ORIGIN).origin]);

/** What the caption under the map states about the drawing, which is where it says it needs no tiles. */
const SCHEMATIC_CAPTION = 'без онлайн-карт';

// A check ends the ride it made: the demonstration refuses to be put back while a rental of a
// person's stands on one of its vehicles, and the checks after it start from the prepared scenario.
test.beforeEach(() => {
  endRidesOf(ACCOUNT_PREFIX);
  restoreScenario();
});

test.afterEach(() => {
  endRidesOf(ACCOUNT_PREFIX);
});

test('the mandatory journey makes no request beyond the local address', async ({ browser }) => {
  const context = await browser.newContext();
  const elsewhere = [];
  await context.route('**/*', (route) => {
    const address = new URL(route.request().url());
    if (ALLOWED_ORIGINS.has(address.origin)) return route.continue();

    elsewhere.push(address.href);
    return route.abort();
  });

  const page = await context.newPage();
  try {
    // The map is the screen nobody signs in for, and it is drawn from the coordinates the
    // application ships: the caption names the scheme, and the markers and the zone are drawn on it.
    await page.goto('/');
    await expect(page.locator('.map-caption')).toContainText(SCHEMATIC_CAPTION);
    await expect(page.locator('.map-canvas .leaflet-interactive').first()).toBeVisible({
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(page.locator('.map-legend-item').first()).toBeVisible();
    assert.ok(
      (await page.locator('.map-canvas .leaflet-interactive').count()) > 1,
      'the map drew nothing of the fleet or the zone',
    );

    // The reservation and the cabinet are the rest of the mandatory journey, and each of them is a
    // screen rather than a document fetched from anywhere else.
    await signUp(page, email('offline'));
    await book(page, await availableModel());
    await expect(page.locator('.reservation-panel-time')).toContainText('Осталось', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await page.getByRole('button', { name: CANCEL_ACTION }).click();
    await page.getByRole('button', { name: CANCEL_ACTION }).last().click();
    await expect(page.locator('.reservation-panel-empty')).toHaveText('Текущей брони нет', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });

    await page.getByRole('link', { name: CABINET_ACTION }).click();
    await expect(page.locator('.cabinet')).toBeVisible({ timeout: RECONCILIATION_PATIENCE_MS });

    assert.deepEqual(elsewhere, [], `the journey reached ${elsewhere.length} address(es) beyond the local one`);
  } finally {
    await context.close();
  }
});

test('the map is drawn from the shipped coordinates, so it needs no tile server', async ({ browser }) => {
  const context = await browser.newContext();
  const tiles = [];
  await context.route('**/*', (route) => {
    const address = new URL(route.request().url());
    if (address.pathname.includes('{z}') || /\/\d+\/\d+\/\d+\.(png|jpg|jpeg|webp)$/.test(address.pathname)) {
      tiles.push(address.href);
    }
    return route.continue();
  });

  const page = await context.newPage();
  try {
    await page.goto('/');
    await expect(page.locator('.map-canvas .leaflet-interactive').first()).toBeVisible({
      timeout: RECONCILIATION_PATIENCE_MS,
    });

    // A tile server is asked for a picture of the world by zoom, column and row. The map is drawn
    // instead from the streets and landmarks the application ships, so none of those is requested.
    assert.deepEqual(tiles, [], `the map asked for ${tiles.length} tile(s)`);
    const drawn = await page.locator('.map-canvas path').count();
    assert.ok(drawn > 1, `the schematic drew ${drawn} path(s)`);
    process.stdout.write(`autonomy: the map drew ${drawn} paths with no tile request\n`);
  } finally {
    await context.close();
  }
});
