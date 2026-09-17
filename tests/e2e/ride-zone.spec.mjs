// What a person sees when they try to end a ride the service will not accept an ending for: the
// browser answers with the service's own refusal, the ride keeps running and keeps being charged for,
// and the same control ends it once the vehicle is back inside the zone.
//
// The refusal is the one no HTTP check can stand in for. An HTTP suite proves the status and the code
// of the answer; what this checks is what a person is told, that the ride is still running afterwards,
// and that the charged time keeps growing while the ending is refused.
//
// A vehicle is placed by hand only while it is standing still, and the model drives a hand-placed
// vehicle back towards its route, so the fleet is stopped for these checks: the ride is held while the
// vehicle is moved, and the duration that must keep growing is the one the service bills for.
import assert from 'node:assert/strict';
import { expect, test } from '@playwright/test';
import { stopSimulator } from '../../scripts/acceptance/simulation.mjs';
import { sql } from '../../scripts/service.mjs';
import { availableModel, book, email, endRidesOf, RECONCILIATION_PATIENCE_MS, signUp } from './person.mjs';
import { restoreScenario, vehicleIdOf } from './scenario.mjs';
import {
  confirmedPositionOf,
  OUTSIDE_ZONE_LONGITUDE,
  placeVehicleAt,
  POSITION_TOLERANCE_DEGREES,
  ZONE_EDGE_LONGITUDE,
  zoneContains,
} from './zones.mjs';

/** The prefix of every account these checks register, which is how their rows are found again. */
const ACCOUNT_PREFIX = 'zone';

/** The controls of a ride, in the words the interface fixes for them. */
const START_ACTION = 'Начать поездку';
const PAUSE_ACTION = 'Пауза';
const RESUME_ACTION = 'Продолжить';
const FINISH_ACTION = 'Завершить поездку';

/** What the panel says once the service confirmed that the ride is over. */
const RIDE_FINISHED = 'Поездка завершена';

/**
 * What the panel says about an ending the service refused because the vehicle is outside the zone: the
 * interface's own two clauses, joined so the line stays inside the length the repository allows. A
 * wording change in the interface is a change to these constants.
 */
const REFUSED_MOVE = 'Автомобиль вне зоны обслуживания';
const REFUSED_RETURN = 'вернитесь в зону и завершите поездку';
const OUTSIDE_ZONE_REFUSAL = `${REFUSED_MOVE}: ${REFUSED_RETURN}`;

/** The panel above the map, which is where every ride of these checks is driven from. */
const PANEL = '.reservation-panel';

/** How long a check waits for the billed time to grow, which is several times one started minute. */
const CHARGE_PATIENCE_MS = 90_000;

// The fleet stands still for these checks, and the demonstration is put back afterwards. The simulator
// is a deliberate part of a demonstration, so it is stopped rather than left as the check found it.
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

test('a refused ending outside the zone is told to the person and the ride keeps being charged for', async ({
  browser,
}) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const model = await availableModel();
  const vehicleId = vehicleIdOf(model);

  try {
    await page.goto('/');
    await signUp(page, email(ACCOUNT_PREFIX));
    await book(page, model);
    await panelAction(page, START_ACTION).click();
    await expect(page.locator('.reservation-panel-time')).toContainText('В режиме');

    // The ride is held so the vehicle stands still, which is the only state a demonstration may place
    // one in: a moving vehicle is where the ride put it.
    await panelAction(page, PAUSE_ACTION).click();
    await expect(page.locator('.reservation-panel-time')).toContainText('Пауза');

    // The vehicle is put just outside the zone's own western edge. The position is read back and put
    // to the zone itself, so what the ending meets is a confirmed fix outside the zone rather than the
    // command that asked for one.
    placeVehicleAt(vehicleId, OUTSIDE_ZONE_LONGITUDE);
    const outside = confirmedPositionOf(vehicleId);
    expect(Math.abs(outside.longitude - OUTSIDE_ZONE_LONGITUDE)).toBeLessThan(POSITION_TOLERANCE_DEGREES);
    expect(zoneContains(outside.longitude, outside.latitude)).toBe(false);

    // The refused ending is charged for from the moment the ride is carried on, so the reading that
    // must grow afterwards is taken from a moving ride on the same rates.
    await panelAction(page, RESUME_ACTION).click();
    await expect(page.locator('.reservation-panel-time')).toContainText('В режиме');
    const beforeRefusal = billedMicroseconds(vehicleId);

    await askToFinish(page);

    // The service refused the ending, and the browser says what it objected to rather than leaving the
    // person to guess. The control that ends the ride is still there, and the ride is still running.
    await expect(page.locator('.reservation-panel-notice')).toHaveText(OUTSIDE_ZONE_REFUSAL);
    await expect(panelAction(page, FINISH_ACTION)).toBeEnabled();
    await expect(page.locator('.reservation-panel-finished')).toHaveCount(0);
    assert.equal(stageOf(vehicleId), 'active', 'the refused ending moved the ride out of its stage');
    assert.equal(openSegments(vehicleId), 1, 'the refused ending closed the segment the ride was on');
    assert.equal(invoicesOf(vehicleId), 0, 'the refused ending issued an invoice');

    // The refusal cost the ride nothing: what the service bills for is still growing.
    await expect
      .poll(() => billedMicroseconds(vehicleId), { timeout: CHARGE_PATIENCE_MS })
      .toBeGreaterThan(beforeRefusal);

    // The vehicle is held again, put back on the zone's edge, and the same control ends the ride there:
    // an ending on the edge is inside the zone, and the invoice is issued for the ride that ran.
    await panelAction(page, PAUSE_ACTION).click();
    await expect(page.locator('.reservation-panel-time')).toContainText('Пауза');
    placeVehicleAt(vehicleId, ZONE_EDGE_LONGITUDE);
    const edge = confirmedPositionOf(vehicleId);
    expect(Math.abs(edge.longitude - ZONE_EDGE_LONGITUDE)).toBeLessThan(POSITION_TOLERANCE_DEGREES);
    expect(zoneContains(edge.longitude, edge.latitude)).toBe(true);
    await askToFinish(page);

    await expect(page.locator('.reservation-panel-time')).toHaveText(RIDE_FINISHED, {
      timeout: RECONCILIATION_PATIENCE_MS,
    });
    assert.equal(stageOf(vehicleId), 'completed', 'the ending on the edge did not complete the ride');
    assert.equal(invoicesOf(vehicleId), 1, 'the ending on the edge issued no single invoice');
  } finally {
    await context.close();
  }
});

/** One control of the panel above the map, which the fleet list has no part of. */
function panelAction(page, name) {
  return page.locator(PANEL).getByRole('button', { name, exact: true });
}

/** Ends the ride through the control and the question the interface asks before it sends one. */
async function askToFinish(page) {
  await panelAction(page, FINISH_ACTION).click();
  await page.locator('.reservation-panel-confirm').getByRole('button', { name: FINISH_ACTION }).click();
}

/** The stage the ride of one vehicle stands in, which a refused ending must leave where it was. */
function stageOf(vehicleId) {
  return sql(`SELECT stage FROM rentals WHERE vehicle_id = '${vehicleId}'`);
}

/**
 * How long the ride of one vehicle has been billed for, in microseconds. It is the duration the billing
 * policy is applied to, so a value that grows is a ride the service still counts; the sum is taken in
 * the database because the panel writes a duration as minutes and seconds.
 */
function billedMicroseconds(vehicleId) {
  return BigInt(
    sql(
      `SELECT coalesce(sum(extract(epoch FROM clock_timestamp() - started_at) * 1000000), 0)::bigint
       FROM ride_segments
       WHERE rental_id = (SELECT id FROM rentals WHERE vehicle_id = '${vehicleId}') AND ended_at IS NULL`,
    ),
  );
}

/** How many segments the ride of one vehicle holds open, which an active ride has exactly one of. */
function openSegments(vehicleId) {
  return Number(
    sql(
      `SELECT count(*) FROM ride_segments
       WHERE rental_id = (SELECT id FROM rentals WHERE vehicle_id = '${vehicleId}') AND ended_at IS NULL`,
    ),
  );
}

/** How many invoices the rides of one vehicle produced, which a refusal must not add to. */
function invoicesOf(vehicleId) {
  return Number(
    sql(`SELECT count(*) FROM invoices WHERE rental_id IN (SELECT id FROM rentals WHERE vehicle_id = '${vehicleId}')`),
  );
}
