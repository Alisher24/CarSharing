import { serverMomentAt, type ServerClock } from './countdown.ts';
import { PACE_TEXT } from './rideCopy.ts';
import type { LiveRental, Progress } from '../../shared/api/current.ts';

/**
 * What a ride in force is doing: the mode it is in, how long it has been in that mode, and what the
 * service's progress statement says about the whole ride. Every value is derived from what the
 * server published — the stored moments, the durations and the amount — and the one that moves is
 * measured against the server's own clock rather than against the local one.
 */

/** The mode a ride can be in. A rental that has not started has no mode yet. */
export type RideMode = 'driving' | 'paused';

/** A rental whose ride has started, which is what the pace of a ride can be read from. */
export type StartedRental = Extract<LiveRental, { state: 'active' | 'paused' }>;

/** A moment of the ride together with the server moment it was published at. */
export type RideMoment = { modeStartedAt: string } & ServerClock;

// The contract states moments in milliseconds the browser can read and durations in whole
// microseconds, so one unit has to be translated into the other to be counted.
const MICROSECONDS_IN_MILLISECOND = 1_000n;

const MICROSECONDS_IN_SECOND = 1_000_000n;

/** How many seconds a minute holds, which is the part a duration is written in after the minutes. */
const SECONDS_IN_MINUTE = 60n;

/** How many digits minutes and seconds each take, so a clock does not change width as it runs. */
const DIGITS_IN_A_PART = 2;

const ZERO = 0n;

/**
 * Whether a rental is a ride that has started. Only such a rental states the mode it is in and the
 * progress of the ride so far; a reservation and a rental that is over state neither.
 */
export function hasStarted(rental: LiveRental): rental is StartedRental {
  return rental.state === 'active' || rental.state === 'paused';
}

/** Which mode a ride in force is in. A rental that has started states one of the two. */
export function rideModeOf(rental: StartedRental): RideMode {
  return rental.state === 'active' ? 'driving' : 'paused';
}

/** What the current mode of a ride is called. */
export function rideModeText(mode: RideMode): string {
  return PACE_TEXT[mode];
}

/**
 * How long a ride has been in its current mode, in whole microseconds, or undefined when the moments
 * it is measured between cannot be read. It is the distance between the moment the server opened the
 * mode and the moment the server is at now, so a tab that was closed for an hour comes back to an
 * hour of the mode rather than to the second it was left at.
 */
export function elapsedInMode(moment: RideMoment, now: Date): bigint | undefined {
  const started = Date.parse(moment.modeStartedAt);
  const serverNow = serverMomentAt(moment, now);
  if (Number.isNaN(started) || serverNow === undefined) return undefined;

  return BigInt(Math.max(serverNow - started, 0)) * MICROSECONDS_IN_MILLISECOND;
}

/** How long the whole ride has been in motion, as the progress statement publishes it. */
export function drivingDuration(progress: Progress): string {
  return progress.driving_duration_microseconds;
}

/** How long the whole ride has been held, as the progress statement publishes it. */
export function pausedDuration(progress: Progress): string {
  return progress.paused_duration_microseconds;
}

/**
 * A duration as minutes and seconds. Microseconds are left out rather than rounded, because the
 * number is the length of an interval that is still running; the digits of the value are never read
 * as a floating-point number, which a long ride would lose.
 */
export function durationText(microseconds: string): string | undefined {
  const duration = wholeMicroseconds(microseconds);
  if (duration === undefined) return undefined;

  const totalSeconds = duration / MICROSECONDS_IN_SECOND;
  const minutes = totalSeconds / SECONDS_IN_MINUTE;
  const seconds = totalSeconds % SECONDS_IN_MINUTE;

  return `${pad(minutes)}:${pad(seconds)}`;
}

function wholeMicroseconds(microseconds: string): bigint | undefined {
  try {
    const duration = BigInt(microseconds);
    return duration < ZERO ? undefined : duration;
  } catch {
    return undefined;
  }
}

function pad(part: bigint): string {
  return part.toString().padStart(DIGITS_IN_A_PART, '0');
}
