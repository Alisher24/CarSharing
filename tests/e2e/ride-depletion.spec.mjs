// What a person sees when a ride ends by itself while their tab is not listening: the tab they come
// back to shows the finished ride, why it ended, and the amount the service fixed for it.
//
// The ending is the one no HTTP check can stand in for. An HTTP suite proves the row the service wrote;
// what this checks is that a browser that was told nothing still arrives at the same result, from the
// report the service holds and the invoice that report names, without the tab that ended the ride
// having sent anything.
//
// The tab is cut off from the private stream rather than from the network: the check is about a change
// that happens while nothing is delivered, and the browser's own offline is what T17's checks cover.
import assert from 'node:assert/strict';
import { expect, test } from '@playwright/test';
import { demoCommand, startSimulator, stopSimulator } from '../../scripts/acceptance/simulation.mjs';
import { somText } from '../../scripts/acceptance/money.mjs';
import { sql } from '../../scripts/service.mjs';
import { availableVehicleOfPowertrain, book, email, endRidesOf, signUp, until } from './person.mjs';
import { restoreScenario } from './scenario.mjs';

/** The prefix of every account these checks register, which is how their rows are found again. */
const ACCOUNT_PREFIX = 'depletion';

/** The powertrain whose vehicle is moved by one source, so spending that source ends its ride. */
const SINGLE_SOURCE_POWERTRAIN = 'electric';

/** The source an electric vehicle is moved by, which the check drains. */
const BATTERY = 'battery';

/** What a source is drained to, which a moving vehicle spends in about a second. */
const DRAINED_TO = '20';

/** The controls of a ride, in the words the interface fixes for them. */
const START_ACTION = 'Начать поездку';

/** What the panel says a ride has in force, which a moving ride is in. */
const IN_MODE = 'В режиме';

/** What the panel says once the service confirmed that the ride is over. */
const RIDE_FINISHED = 'Поездка завершена';

/** Why the panel says the ride ended, and which source ran out to end it. */
const DEPLETION_REASON = 'закончился запас энергии или топлива';
const DEPLETED_SOURCE = 'батарея';

/** Where the panel states what the ride cost, and what it writes before the amount. */
const INVOICE_TOTAL = 'Итог счёта';

/** Where the private stream of the signed-in person lives, which one check interrupts. */
const PRIVATE_EVENTS = '**/api/v1/me/events';

/** The address that switches the periodic reconciliation off, which only a test asks for. */
const WITHOUT_RECONCILIATION = '/?reconcile=off';

/** How long the model may take to reach the ending, which is several times its own second. */
const DEPLETION_PATIENCE_MS = 30_000;

/** How long a change may take to appear once the tab can read again, which is one stream retry more. */
const RECONCILIATION_PATIENCE_MS = 30_000;

/** How long the tab is left unable to hear anything, which is several times the delivery bound. */
const BLIND_MILLISECONDS = 15_000;

/**
 * How many subscriptions the tab is answered before every later attempt is refused. The first one is
 * what makes the tab a client that was connected: a tab that never subscribed would show the same
 * screen for the wrong reason, and one whose every attempt was refused would prove nothing about a
 * signal it never had.
 */
const SUBSCRIPTIONS_ANSWERED = 1;

/**
 * The handshake a subscription is answered with, which is what the server sends first: the stated
 * retry delay and the moment of the handshake. A stream that ends there is a subscription that was
 * established and then lost, which is the state this check needs the tab to be in.
 */
const HANDSHAKE = `retry: 3000\nevent: ready\ndata: {"server_time":"${new Date().toISOString()}"}\n\n`;

// The check drives a vehicle the model spends, so the fleet is advanced by a process that is stopped
// again afterwards: the simulator is a deliberate part of a demonstration rather than something a
// running stack starts.
test.beforeEach(() => {
  endRidesOf(ACCOUNT_PREFIX);
  restoreScenario();
  stopSimulator();
  // Every check registers its own account, and the checks share one address. Clearing the counters is
  // the harness standing in for the passage of time, which is also how access returns in production.
  sql('DELETE FROM rate_limit_counters');
});

test.afterEach(() => {
  endRidesOf(ACCOUNT_PREFIX);
  restoreScenario();
});

test('a ride that ends while the tab hears nothing is shown finished with its reason and amount', async ({
  browser,
}) => {
  const context = await browser.newContext();
  // Only the first subscription is answered, and every attempt after it is refused: the tab hears the
  // ride it started and nothing that happens while the ride ends. The address switches the periodic
  // reconciliation off, so what the panel shows afterwards is what reading brought, not a poll that
  // would have found the ending anyway — the interval the application ships is left alone.
  let subscriptions = 0;
  await context.route(PRIVATE_EVENTS, (route) => {
    subscriptions += 1;
    if (subscriptions > SUBSCRIPTIONS_ANSWERED) return route.abort();
    return route.fulfill({ status: 200, contentType: 'text/event-stream', body: HANDSHAKE });
  });

  const page = await context.newPage();
  const vehicle = await availableVehicleOfPowertrain(SINGLE_SOURCE_POWERTRAIN);
  const vehicleId = vehicle.id;

  try {
    await page.goto(WITHOUT_RECONCILIATION);
    await signUp(page, email(ACCOUNT_PREFIX));
    await book(page, vehicle.model);
    await page.getByRole('button', { name: START_ACTION }).click();
    await expect(page.locator('.reservation-panel-time')).toContainText(IN_MODE);

    // The ride is emptied while the model is running, and the service ends it on the first tick that
    // finds no usable source.
    startSimulator();
    demoCommand('set-energy-remaining', '--vehicle', vehicleId, '--source', BATTERY, '--remaining', DRAINED_TO);

    const ended = await until(
      () => storedEnding(rentalIdOf(vehicleId)),
      'the ride whose source ran out was never ended',
      DEPLETION_PATIENCE_MS,
    );
    assert.equal(ended.reason, 'energy_depleted', 'the ride was not ended by the source running out');
    assert.deepEqual(ended.sources, [BATTERY]);
    const total = invoiceTotalOf(vehicleId);

    // The tab the person comes back to still shows the ride it last heard about: the ending below is
    // what reading produces, not what a signal delivered.
    await page.waitForTimeout(BLIND_MILLISECONDS);
    expect(subscriptions, 'the tab never subscribed, so nothing was ever missed').toBeGreaterThan(
      SUBSCRIPTIONS_ANSWERED,
    );
    await expect(page.locator('.reservation-panel-time')).toContainText(IN_MODE);

    // The subscription is answered again. The tab reads what the service holds rather than what it was
    // told, and the finished ride appears with the reason the service stored and the amount it fixed.
    await context.unroute(PRIVATE_EVENTS);
    await expect(page.locator('.reservation-panel-time')).toHaveText(RIDE_FINISHED, {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    await expect(page.locator('.ride-progress')).toContainText(DEPLETION_REASON);
    await expect(page.locator('.ride-progress')).toContainText(DEPLETED_SOURCE);
    await expect(page.locator('.ride-progress')).toContainText(INVOICE_TOTAL);
    await expect(page.locator('.ride-progress')).toContainText(somText(total));
  } finally {
    stopSimulator();
    await context.close();
  }
});

/** The rental the ride of one vehicle is, which is the only one that vehicle has. */
function rentalIdOf(vehicleId) {
  const id = sql(`SELECT id FROM rentals WHERE vehicle_id = '${vehicleId}'`);
  if (id === '') throw new Error(`the vehicle ${vehicleId} holds no rental`);
  return id;
}

/** The moment one ride ended and why, as the rental row states them, or nothing while it has not. */
function storedEnding(rentalId) {
  const row = sql(
    `SELECT coalesce(completion_reason, '') || '|' || coalesce(array_to_string(exhausted_sources, ','), '')
     FROM rentals WHERE id = '${rentalId}' AND ended_at IS NOT NULL`,
  );
  if (row === '') return undefined;
  const [reason, sources] = row.split('|');
  return { reason, sources: sources === '' ? [] : sources.split(',') };
}

/**
 * The total of the one invoice the ride of a vehicle produced, as the exact string it was issued at.
 */
function invoiceTotalOf(vehicleId) {
  const total = sql(`SELECT total_amount_tyiyn FROM invoices WHERE rental_id = '${rentalIdOf(vehicleId)}'`);
  if (total === '') throw new Error(`the ride of ${vehicleId} was not invoiced`);
  return total;
}
