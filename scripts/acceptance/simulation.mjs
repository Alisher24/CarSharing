// The simulated fleet on the assembled stack: the tick the simulator calls, the set-to-value commands
// of the demonstration control, and the model the database holds between them.
//
// Both internal operations are reached the way the services reach them — through the client
// containers, over the internal network — because the proxy publishes neither: `/internal` answers an
// unknown resource from outside, which is what makes the surface closed rather than merely
// unadvertised. A command here therefore states the arguments of the command line, not a request.
import { compose, sql } from '../service.mjs';

/** The identifier of the demonstration vehicle the checks below drive, which the seed numbers first. */
export const FIRST_VEHICLE = '01994342-6ba7-7000-8000-000100000001';

/**
 * Advances the fleet once, under `tickId` when one is given, and answers what the tick reported. The
 * tick is run as a one-off container rather than waited for, so a check states the moment it advances
 * to rather than racing the clock the simulator keeps.
 */
export function tickOnce(tickId) {
  const arguments_ = ['run', '--rm', 'simulator', '-once'];
  if (tickId !== undefined) {
    arguments_.push('-tick-id', tickId);
  }
  return lastJsonLine(compose(...arguments_));
}

/**
 * Runs one set-to-value command of the demonstration control and answers what the API replied.
 * `arguments_` is the command line: the action and its flags.
 */
export function demoCommand(...arguments_) {
  return lastJsonLine(compose('--profile', 'demo', 'run', '--rm', 'democontrol', ...arguments_));
}

/** Stops the simulator, which is how a check makes the fleet stand still. */
export function stopSimulator() {
  compose('stop', 'simulator');
}

/** Starts the simulator again, which is how a check resumes the fleet. */
export function startSimulator() {
  compose('start', 'simulator');
}

/**
 * Reads the model of one vehicle: the route it travels, how far along it stands, whether it has run
 * out and the moment everything above describes. A vehicle the model has not met has no row, which is
 * `undefined` here rather than an empty reading.
 */
export function storedModel(vehicleId) {
  const row = sql(
    `SELECT route_id || '|' || path || '|' || join_path || '|' || longitude || '|' || latitude || '|' ||
            ${storedMoment('processed_at')} || '|' || depleted || '|' || is_off_route
     FROM simulation_states WHERE vehicle_id = '${vehicleId}'`,
  );
  if (row === '') {
    return undefined;
  }
  const [routeId, path, joinPath, longitude, latitude, processedAt, depleted, offRoute] = row.split('|');
  return {
    routeId,
    path,
    joinPath,
    longitude,
    latitude,
    processedAt,
    depleted: depleted === 'true',
    offRoute: offRoute === 'true',
  };
}

/** Reads the charge of one source of a vehicle, which is what the model spends as it moves. */
export function storedCharge(vehicleId, sourceKind) {
  return sql(
    `SELECT charged FROM simulation_sources
     WHERE vehicle_id = '${vehicleId}' AND source_kind = '${sourceKind}'`,
  );
}

/** Reports whether a vehicle has been taken out of service. */
export function serviceRequired(vehicleId) {
  return sql(`SELECT service_required FROM vehicles WHERE id = '${vehicleId}'`) === 't';
}

/** Reads the moment a ride ended and why, as the rental row states them. */
export function storedEnding(rentalId) {
  const row = sql(
    `SELECT coalesce(${storedMoment('ended_at')}, '') || '|' || coalesce(completion_reason, '') || '|' ||
            coalesce(array_to_string(exhausted_sources, ','), '')
     FROM rentals WHERE id = '${rentalId}'`,
  );
  if (row === '' || row === '||') {
    return undefined;
  }
  const [endedAt, reason, exhausted] = row.split('|');
  return { endedAt, reason, sources: exhausted === '' ? [] : exhausted.split(',') };
}

/** Reads the report of one finished ride: the moment it ended, why, and the invoice it produced. */
export function completionNotification(rentalId) {
  const row = sql(
    `SELECT coalesce(${storedMoment('ended_at')}, '') || '|' || coalesce(completion_reason, '') || '|' ||
            coalesce(array_to_string(exhausted_sources, ','), '') || '|' || invoice_id::text
     FROM notifications WHERE rental_id = '${rentalId}' AND kind = 'rental_completed'`,
  );
  if (row === '') {
    return undefined;
  }
  const [endedAt, reason, exhausted, invoiceId] = row.split('|');
  return { endedAt, reason, sources: exhausted === '' ? [] : exhausted.split(','), invoiceId };
}

/** Reads the moment one invoice was issued at. */
export function issuedAt(rentalId) {
  return sql(`SELECT ${storedMoment('issued_at')} FROM invoices WHERE rental_id = '${rentalId}'`);
}

/** Reads what the next payment attempt of one ride is asked to decide, if anything is. */
export function demandedOutcome(rentalId) {
  const row = sql(`SELECT outcome FROM demo_payment_outcomes WHERE rental_id = '${rentalId}'`);
  return row === '' ? undefined : row;
}

/** Counts the invoices of one ride, which is how a second ending would show itself. */
export function invoicesOf(rentalId) {
  return Number(sql(`SELECT count(*) FROM invoices WHERE rental_id = '${rentalId}'`));
}

/** Counts the reports of one finished ride. */
export function completionsOf(rentalId) {
  return Number(
    sql(`SELECT count(*) FROM notifications WHERE rental_id = '${rentalId}' AND kind = 'rental_completed'`),
  );
}

/** Counts the payment attempts one invoice is owed. */
export function attemptsOwed(invoiceId) {
  return Number(sql(`SELECT count(*) FROM outbox WHERE kind = 'payment.attempt' AND resource_id = '${invoiceId}'`));
}

/** One stored moment rendered the way the contract publishes one. */
function storedMoment(column) {
  return `to_char(${column} AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`;
}

/**
 * Waits until `reached` answers something truthy, and fails with `complaint` when it never does. The
 * model advances on the simulator's own second, so a check that reads it waits for it rather than
 * assuming how long a tick took.
 */
export async function until(reached, complaint, patienceMs = 20_000) {
  const deadline = Date.now() + patienceMs;
  for (;;) {
    const answer = await reached();
    if (answer) {
      return answer;
    }
    if (Date.now() > deadline) {
      throw new Error(complaint);
    }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
}

/**
 * Reads the last line of a command's output as JSON. The clients log their result as JSON, and a
 * container that prints more than that — the identifier of the run, a warning — must not make the
 * answer unreadable.
 */
function lastJsonLine(output) {
  const lines = output.split('\n').filter((line) => line.trim() !== '');
  if (lines.length === 0) {
    return undefined;
  }
  return JSON.parse(lines.at(-1));
}
