import type { CurrentSnapshot, Rental } from '../../shared/api/current.ts';
import { answersOf } from '../../shared/read/documentAnswers.ts';
import type { AnswerHandlers, DocumentKind } from '../../shared/read/readCycle.ts';
import { versionsOf } from '../../shared/read/observedResource.ts';
import { isNewerTimestamp } from '../../shared/read/version.ts';

/** What one session of reading the account's own rental knows, which one answer needs to store. */
export type CurrentSession = {
  session: string;
  reading: { current: CurrentSnapshot | undefined };
};

/**
 * currentAnswers builds the answers of the coordinator that reads the account's own rental in force.
 * The public resources are answered by nobody here: a private read never publishes them, so a signal
 * about one is left to the catalog that reads it.
 *
 * The notifications addressed to the same account, its invoices and the history of the rides it has
 * finished are read by coordinators of their own rather than by this one: each of those is ordered
 * and versioned by entry rather than by the moment a snapshot was computed at, and a signal about
 * one must not decide when the rental in force is read — nor the other way round.
 */
export function currentAnswers(state: CurrentSession): Record<DocumentKind, AnswerHandlers> {
  return answersOf({ rentals: currentAnswer(state) });
}

/**
 * The answer of the private resource. An answer is stored only when it was computed at a later
 * moment than the one already held, so a late answer cannot put a cancelled reservation back on
 * screen, and an answer of another account is refused before it is looked at.
 *
 * The versions an answer covers are the ones it publishes. An answer that names no rental covers
 * nothing, and the change that asked for it stays remembered until an answer names that rental —
 * which costs a repeated read of one resource and never a wrong screen.
 */
function currentAnswer(state: CurrentSession): AnswerHandlers {
  return {
    accepts: (session) => session === state.session,
    observe: (value, session) => {
      if (session !== state.session) return undefined;

      const snapshot = value as CurrentSnapshot;
      if (!updatesCurrent(state.reading.current, snapshot)) return undefined;

      state.reading.current = snapshot;
      return { value: snapshot, covered: versionsOf(rentalsOf(snapshot)) };
    },
  };
}

/** Whether one answer is worth storing over the one already held. */
export function updatesCurrent(stored: CurrentSnapshot | undefined, incoming: CurrentSnapshot): boolean {
  if (stored === undefined) return true;
  return isNewerTimestamp(incoming.server_time, stored.server_time);
}

/** The rental one answer names, or none when it says nothing is current. */
function rentalsOf(snapshot: CurrentSnapshot): Rental[] {
  return snapshot.kind === 'rental' ? [snapshot.rental] : [];
}
