// Where the seeded service zone lies, and how a check moves a vehicle across its edge. The zone is
// the demonstration's own rectangle, and the two longitudes below are the same latitude on either
// side of its western edge: the one place a check can stand to be refused and then allowed without
// travelling anywhere.
//
// The move goes through the demonstration control, which is the product's own way of placing a
// vehicle by hand: a check that wrote the position with SQL would show a person a fix the service
// never confirmed, and the ending is decided on confirmed telemetry. A coordinate crosses the wire as
// the contract's own number type, which carries about seven significant digits, so a check reads the
// position back and compares it rather than expecting the number it sent.
import { sql } from '../../scripts/service.mjs';
import { demoCommand } from '../../scripts/acceptance/simulation.mjs';

/** The western edge of the seeded zone, which an ending on it is allowed. */
export const ZONE_EDGE_LONGITUDE = 74.55;

/** A longitude just outside that edge, which is what a check places a vehicle at. */
export const OUTSIDE_ZONE_LONGITUDE = 74.5499;

/** A latitude inside the zone's own span, so the two longitudes differ in one coordinate alone. */
export const ZONE_LATITUDE = 42.87;

/**
 * How far a confirmed position may lie from the one a command asked for. The contract writes a
 * coordinate as a number of about seven significant digits, so a position is confirmed to within a
 * ten-thousandth of a degree — about ten metres — of the place it was put.
 */
export const POSITION_TOLERANCE_DEGREES = 1e-4;

/** Places one vehicle at a longitude on the zone's own latitude, through the demonstration control. */
export function placeVehicleAt(vehicleId, longitude) {
  demoCommand(
    'set-position',
    '--vehicle',
    vehicleId,
    '--longitude',
    String(longitude),
    '--latitude',
    String(ZONE_LATITUDE),
  );
}

/**
 * The position the service confirmed for one vehicle, read back as the two numbers a check compares.
 * A vehicle that has never reported has no reading, which is an error here rather than an empty one.
 */
export function confirmedPositionOf(vehicleId) {
  const reading = sql(
    `SELECT st_x(position) || '|' || st_y(position) FROM vehicle_telemetry WHERE vehicle_id = '${vehicleId}'`,
  );
  if (reading === '') throw new Error(`the service confirmed no position for the vehicle ${vehicleId}`);

  const [longitude, latitude] = reading.split('|');
  return { longitude: Number(longitude), latitude: Number(latitude) };
}

/**
 * Whether the seeded service zone contains one point, which is the very predicate the service applies
 * to a position before it accepts an ending. A check asks the database this question rather than
 * comparing coordinates textually, so where the zone's edge really lies is answered by the geometry.
 */
export function zoneContains(longitude, latitude) {
  return (
    sql(`SELECT st_covers(area, st_setsrid(st_makepoint(${longitude}, ${latitude}), 4326)) FROM service_zones`) === 't'
  );
}
