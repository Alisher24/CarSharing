// The demonstration state a browser check starts from, and the two changes it makes. Everything that
// reaches the stack belongs to the helpers the acceptance suites already use — the restoration, and
// the deadline the transition is due at — so neither is written a second time here.
import { sql } from '../../scripts/service.mjs';
import { moveDeadline } from '../../scripts/acceptance/reservations.mjs';

/** The vehicles the prepared scenario holds, as the seed names them. */
export const RESERVED_VEHICLE_MODEL = 'Демо Бензин 3';

/** How far into the past a reservation's deadline is moved to make its expiry due. */
const DEADLINE_IN_THE_PAST_SECONDS = -1;

/** Puts the prepared scenario back, as the demonstration declares it and the acceptance suites do. */
export { restoreScenario } from '../../scripts/acceptance/fleet.mjs';

/** The identifier of one vehicle, read by the model name the interface displays. */
export function vehicleIdOf(model) {
  const id = sql(`SELECT id FROM vehicles WHERE model = '${model}'`);
  if (id === '') throw new Error(`the demonstration has no vehicle named ${model}`);
  return id;
}

/** The public state the service publishes for one vehicle, read the way a client learns it. */
export function publishedStatusOf(vehicleId) {
  return sql(`
    SELECT coalesce(live.stage, 'free')
    FROM vehicles vehicle
    LEFT JOIN rentals live ON live.vehicle_id = vehicle.id AND live.ended_at IS NULL
    WHERE vehicle.id = '${vehicleId}'`);
}

/**
 * Makes the expiry of the reservation holding one vehicle due by moving its deadline into the past,
 * so the deadline pass of the worker performs the transition exactly as it does in production: the
 * rule is the rentals module's, and a check that wrote the transition itself would be a second
 * declaration of it.
 */
export function expireReservationOf(vehicleId) {
  const rentalId = sql(`SELECT id FROM rentals WHERE vehicle_id = '${vehicleId}' AND ended_at IS NULL`);
  if (rentalId === '') throw new Error(`no live reservation held the vehicle ${vehicleId}`);
  moveDeadline(rentalId, DEADLINE_IN_THE_PAST_SECONDS);
}
