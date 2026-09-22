/**
 * How much of a reservation is left. It is derived from the deadline the server stored, the moment
 * the answer that carried it was computed at, and the moment that answer arrived here — never from a
 * count of ticks that have passed since, which a suspended tab or a lost connection would make wrong.
 */

import type { ServerClock } from './serverClock.ts';

/** A reminder of the deadline as one value: what the server said, and when it said it. */
export type Deadline = {
  /** The moment the reservation runs out, as the contract writes it. */
  expiresAt: string;
} & ServerClock;

/**
 * How often a countdown on screen is recomputed. It is a redraw rather than a count: the value comes
 * from the deadline, the moment the server computed its answer at and the moment that answer arrived,
 * so a tick that never happened costs a late redraw and nothing else.
 */
export const COUNTDOWN_TICK_MILLISECONDS = 1_000;

/** What the remaining time of a reservation is, and how it is written. */
export type Countdown =
  { state: 'left'; milliseconds: number; text: string } | { state: 'due' } | { state: 'unreadable' };

/**
 * countdownAt reports the time left at one moment. A deadline that has been reached is reported as
 * due rather than as a negative time: the interface then waits for the server to confirm the
 * release instead of declaring the vehicle free by itself.
 */
export function countdownAt(deadline: Deadline, now: Date): Countdown {
  const expires = momentOf(deadline.expiresAt);
  const computed = momentOf(deadline.serverTime);
  if (expires === undefined || computed === undefined) return { state: 'unreadable' };

  // The server's own interval is the truth; the local clock only says how long ago that answer
  // arrived. A tab that was suspended for an hour therefore comes back to a smaller countdown
  // rather than to one that carried on from where it stopped.
  const elapsed = now.getTime() - deadline.receivedAt.getTime();
  const milliseconds = expires - computed - elapsed;
  if (milliseconds <= 0) return { state: 'due' };

  return { state: 'left', milliseconds, text: remainingText(milliseconds) };
}

/** The longest a countdown is shown over: minutes and seconds of the remaining time. */
export function remainingText(milliseconds: number): string {
  const total = Math.ceil(milliseconds / 1_000);
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${minutes}:${String(seconds).padStart(2, '0')}`;
}

// A moment the contract wrote is read as the instant it names; microseconds are below what a
// countdown distinguishes.
function momentOf(wireMoment: string): number | undefined {
  const parsed = Date.parse(wireMoment);
  return Number.isNaN(parsed) ? undefined : parsed;
}
