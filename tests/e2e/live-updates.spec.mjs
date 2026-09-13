// What a person sees when the fleet changes: a change committed in the database reaches a second
// connected browser in under two seconds, and a client whose stream is broken keeps working and says
// so instead of showing a fleet that quietly stopped being true.
//
// The periodic reconciliation is switched off for the first check by the address it loads, so what
// the check measures is the delivered signal rather than a poll that would have found the change
// anyway. The second check leaves the shipped interval alone.
import { expect, test } from '@playwright/test';
import {
  expireReservationOf,
  publishedStatusOf,
  RESERVED_VEHICLE_MODEL,
  restoreScenario,
  vehicleIdOf,
} from './scenario.mjs';

/** The bound a committed change must reach a connected client within. */
const DELIVERY_BOUND_MS = 2000;

/** The address that switches the periodic reconciliation off, which only a test asks for. */
const WITHOUT_RECONCILIATION = '/?reconcile=off';

/** How long a change may take to appear through reconciliation alone. */
const RECONCILIATION_PATIENCE_MS = 20_000;

const DELAYED_NOTICE = 'Обновления задерживаются';

function rowOf(page, model) {
  return page.locator('.fleet-row', { has: page.locator('.fleet-row-model', { hasText: model }) });
}

function statusOf(page, model) {
  return rowOf(page, model).locator('.fleet-row-status');
}

test.beforeEach(() => {
  restoreScenario();
});

test('a change reaches a second client within two seconds of the commit', async ({ browser }) => {
  const vehicleId = vehicleIdOf(RESERVED_VEHICLE_MODEL);

  // Two browser contexts rather than two tabs: tabs of one profile share a cookie, a connection
  // pool and a service worker, and would not be two clients.
  const first = await browser.newContext();
  const second = await browser.newContext();
  const other = await first.newPage();
  const watching = await second.newPage();

  try {
    await Promise.all([other.goto(WITHOUT_RECONCILIATION), watching.goto(WITHOUT_RECONCILIATION)]);

    // Both clients show the vehicle as the scenario prepared it, and both streams are established
    // before the change is made: a signal is not replayed, so a client that connected later would
    // prove nothing.
    for (const page of [other, watching]) {
      await expect(statusOf(page, RESERVED_VEHICLE_MODEL)).toHaveAttribute('data-status', 'reserved');
    }
    await Promise.all([expectNoNotice(other), expectNoNotice(watching)]);

    const committedAt = expireReservationOf(vehicleId);
    expect(publishedStatusOf(vehicleId)).not.toBe('reserved');

    await expect(statusOf(watching, RESERVED_VEHICLE_MODEL)).not.toHaveAttribute('data-status', 'reserved');
    const arrivedIn = Date.now() - committedAt;
    expect(arrivedIn, `the change reached the second client ${arrivedIn} ms after the commit`).toBeLessThan(
      DELIVERY_BOUND_MS,
    );
  } finally {
    await first.close();
    await second.close();
  }
});

test('a client whose stream is broken says so and keeps repairing itself', async ({ browser }) => {
  const vehicleId = vehicleIdOf(RESERVED_VEHICLE_MODEL);
  const context = await browser.newContext();

  // The first subscription is answered and then ends, which is what a client sees when the stream
  // breaks; every later attempt is refused, so the client stays in the degraded state the check is
  // about instead of reconnecting successfully.
  let answered = false;
  await context.route('**/api/v1/events', async (route) => {
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
    await page.goto('/');

    // The map is loaded from REST, and the broken stream is stated rather than hidden.
    await expect(rowOf(page, RESERVED_VEHICLE_MODEL)).toBeVisible();
    await expect(statusOf(page, RESERVED_VEHICLE_MODEL)).toHaveAttribute('data-status', 'reserved');
    await expect(page.locator('.stream-notice')).toHaveText(new RegExp(DELAYED_NOTICE));

    expireReservationOf(vehicleId);

    // The signal never arrives, so what shows the change is the reconciliation the shipped
    // application runs while it is open.
    await expect(statusOf(page, RESERVED_VEHICLE_MODEL)).not.toHaveAttribute('data-status', 'reserved', {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
  } finally {
    await context.close();
  }
});

/** Waits until the application reports its subscription as established. */
async function expectNoNotice(page) {
  await expect(page.locator('.stream-notice')).toHaveCount(0);
}
