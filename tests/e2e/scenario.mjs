// The demonstration state a browser check starts from, and the one change it makes. Both speak to
// the database the assembled stack runs on, because the HTTP commands that will perform these
// transitions belong to later tasks and a check of the delivery path must not wait for them.
import { compose, sql } from '../../scripts/service.mjs';

/** The vehicles the prepared scenario holds, as the seed names them. */
export const RESERVED_VEHICLE_MODEL = 'Демо Бензин 3';

/**
 * Puts the prepared scenario back, so a check starts from the state the demonstration declares: the
 * reserved vehicle it looks for is reserved again, and its version sequence moves forward rather
 * than starting over.
 */
export function restoreScenario() {
  compose('--profile', 'demo', 'run', '--rm', 'demo-scenario');
}

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
 * Ends the reservation that holds one vehicle, raises the version of the rental and of the vehicle,
 * and records the signals of both — in one transaction, exactly as the rentals module performs it.
 *
 * The statement states the instant the database committed the change, which is what a delivery is
 * measured from: measuring from the moment the command was sent would credit the delivery with the
 * time the change itself took to be made.
 */
export function expireReservationOf(vehicleId) {
  const committed = sql(`
    WITH expired AS (
        UPDATE rentals
        SET stage = 'expired', ended_at = expires_at, version = version + 1
        WHERE vehicle_id = '${vehicleId}' AND ended_at IS NULL
        RETURNING id, vehicle_id, user_id, version
    ), raised AS (
        UPDATE vehicles
        SET version = vehicles.version + 1
        FROM expired
        WHERE vehicles.id = expired.vehicle_id
        RETURNING vehicles.id, vehicles.version
    ), recorded AS (
        INSERT INTO outbox (kind, resource_id, version, recipient_id)
        SELECT 'vehicle.changed', id, version, NULL FROM raised
        UNION ALL
        SELECT 'rental.changed', id, version, user_id FROM expired
        RETURNING 1
    )
    SELECT (extract(epoch FROM clock_timestamp()) * 1000)::bigint`);
  if (committed === '') throw new Error(`no live reservation held the vehicle ${vehicleId}`);
  return Number(committed);
}
