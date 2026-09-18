// What the mandatory journey costs the network: the map and every screen a demonstration needs work
// without a single request beyond the local address, and the browser makes none.
//
// Localhost-only is the claim a person cannot check by looking at the screen, which is why it is
// checked here. The public address may also be reached on the loopback address of the machine, and
// every other request is refused rather than merely observed: a page that quietly waited for a tile
// server would then hang instead of passing on a slow one, so the check fails on what the page asked
// for rather than on how long it took.
//
// The map underneath the fleet is drawn from the installation's own archive of vector tiles, so the
// second check reads back what the map asked for: every tile, glyph and sprite came from the address
// the application was served from, and none of them from a tile provider.
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

/** The path the basemap is served under, which is where the archive, the glyphs and the sprite live. */
const BASEMAP_PREFIX = '/basemap/';

/** What the caption under the map names, which is the drawing and the sources it was drawn from. */
const BASEMAP_CAPTION = 'Карта Бишкека';

/** What the map must credit on itself, which is the condition the data is published under. */
const ATTRIBUTION_SOURCE = 'OpenStreetMap';

/** What the map says when the archive underneath it could not be read. */
const BASEMAP_ABSENCE = 'Подложка карты недоступна';

/** One vehicle drawn on the map, which is what a person clicks to open its card. */
const VEHICLE_MARKER = '.map-vehicle-marker';

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
    // The map is the screen nobody signs in for, and it is drawn from the archive the installation
    // serves: the caption names the drawing, and the markers and the zone are drawn on it.
    await page.goto('/');
    await expect(page.locator('.map-caption')).toContainText(BASEMAP_CAPTION);
    await expect(page.locator('.map-canvas')).toHaveAttribute('data-basemap', 'ready', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(page.locator('.map-legend-item').first()).toBeVisible();
    assert.ok((await page.locator(VEHICLE_MARKER).count()) > 1, 'the map drew nothing of the fleet');

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

test('every tile, glyph and sprite of the map comes from the installation itself', async ({ browser }) => {
  const context = await browser.newContext();
  const mapRequests = [];
  await context.route('**/*', (route) => {
    const address = new URL(route.request().url());
    // A tile provider is asked for a picture of the world by zoom, column and row; the basemap is
    // asked for the archive, a range of code points and the sprite, all below one path of this build.
    const fromBasemap = address.pathname.includes(BASEMAP_PREFIX);
    const fromProvider =
      address.pathname.includes('{z}') || /\/\d+\/\d+\/\d+\.(png|jpg|jpeg|webp)$/.test(address.pathname);
    if (fromBasemap || fromProvider) {
      mapRequests.push({ href: address.href, origin: address.origin, fromProvider });
    }
    return route.continue();
  });

  const page = await context.newPage();
  try {
    await page.goto('/');
    await expect(page.locator('.map-canvas')).toHaveAttribute('data-basemap', 'ready', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(page.locator(VEHICLE_MARKER).first()).toBeVisible();

    assert.ok(mapRequests.length > 0, 'the map asked for nothing at all');
    const beyond = mapRequests.filter((request) => request.origin !== new URL(SERVICE_ORIGIN).origin);
    assert.deepEqual(beyond, [], `the map reached ${beyond.length} address(es) beyond the installation`);
    const tiled = mapRequests.filter((request) => request.fromProvider);
    assert.deepEqual(tiled, [], `the map asked a tile provider for ${tiled.length} tile(s)`);

    const asked = mapRequests.map((request) => new URL(request.href).pathname);
    assert.ok(
      asked.some((path) => path.endsWith('.pmtiles')),
      'the map never read the archive',
    );
    process.stdout.write(
      `autonomy: the map asked for ${asked.length} addresses, all of them local: ${asked.join(', ')}\n`,
    );
  } finally {
    await context.close();
  }
});

test('the map is ready, marked, credited and clickable', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await page.goto('/');
    await expect(page.locator('.map-canvas')).toHaveAttribute('data-basemap', 'ready', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });

    // The fleet is on the map as elements a person can see and press, and the legend explains them.
    const markers = page.locator(VEHICLE_MARKER);
    await expect(markers.first()).toBeVisible();
    assert.ok((await markers.count()) > 1, 'the map drew one marker or none');
    await expect(page.locator('.map-legend-item').first()).toBeVisible();

    // The data is published under a licence that asks to be credited where it is shown.
    await expect(page.locator('.maplibregl-ctrl-attrib')).toContainText(ATTRIBUTION_SOURCE);

    // A marker opens the card of its vehicle, which is how a person books one from the map.
    const model = await markers.first().getAttribute('aria-label');
    await markers.first().click();
    await expect(page.locator('.vehicle-card-model')).toBeVisible();
    assert.ok(model !== null && model.length > 0, 'a marker names no vehicle');
    await expect(page.locator('.vehicle-card-model')).toContainText(model.split(' · ')[0]);
  } finally {
    await context.close();
  }
});

test('an archive that cannot be read is explained rather than left blank', async ({ browser }) => {
  const context = await browser.newContext();
  await context.route('**/*.pmtiles', (route) => route.abort('failed'));

  const page = await context.newPage();
  try {
    await page.goto('/');

    // The map says what is missing, and everything that does not come from the archive is still
    // there: the fleet and its legend do not depend on the basemap.
    await expect(page.locator('.map-canvas')).toHaveAttribute('data-basemap', 'unavailable', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(page.locator('.map-basemap-notice')).toContainText(BASEMAP_ABSENCE);
    await expect(page.locator(VEHICLE_MARKER).first()).toBeVisible({
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(page.locator('.map-legend-item').first()).toBeVisible();
  } finally {
    await context.close();
  }
});
