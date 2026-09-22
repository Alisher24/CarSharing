import type { CurrentSnapshot, Rental } from '../api/current.ts';

/**
 * The moment the server stated together with the local moment its answer arrived. Every interval the
 * interface shows is measured between two such moments; the difference between them is what turns a
 * stored moment into the moment it names now.
 */
export type ServerClock = {
  /** The moment the answer was computed at, as the contract writes it. */
  serverTime: string;

  /** The local moment that answer arrived, which is what a countdown is anchored to. */
  receivedAt: Date;
};

/**
 * serverMomentAt is the moment the server would state now, or undefined when either the moment it
 * did state or the local clock cannot be read. The server's own moment is the truth; the local clock
 * only says how long ago that answer arrived, so a tab that was suspended for an hour reads the
 * server's hour as passed rather than as a count of the ticks it managed to run.
 */
export function serverMomentAt(clock: ServerClock, now: Date): number | undefined {
  const computed = momentOf(clock.serverTime);
  if (computed === undefined) return undefined;

  return computed + (now.getTime() - clock.receivedAt.getTime());
}

// A moment the contract wrote is read as the instant it names. Milliseconds are what a browser can
// hold; the microseconds the wire format carries are below what this display distinguishes.
function momentOf(wireMoment: string): number | undefined {
  const parsed = Date.parse(wireMoment);
  return Number.isNaN(parsed) ? undefined : parsed;
}

/** The rental a current answer holds, or undefined when nothing is current. */
export function currentRental(snapshot: CurrentSnapshot | undefined): Rental | undefined {
  return snapshot?.kind === 'rental' ? snapshot.rental : undefined;
}
