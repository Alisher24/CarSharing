import type { ObservedVersions } from './changes.ts';

/**
 * One resource a reader keeps up to date, which is what a change signal and a ready frame address.
 * The catalog reads the public ones; the account's own reservation is read by the reader of the
 * private stream, and neither coordinator serves a document that belongs to the other.
 */
export type DocumentKind = 'vehicles' | 'zones' | 'tariffs' | 'current';

/**
 * What one resource does with a REST answer. The handlers are asked twice about one answer: once
 * for whether it belongs to the current session at all, and once for what storing it would mean.
 */
export type AnswerHandlers = {
  /** Whether this answer belongs to the session a person is looking at. */
  accepts: (session: string) => boolean;

  /** The value to store, and the versions this answer covers; or nothing when it stores none. */
  observe: (value: unknown, session: string) => { value: unknown; covered: ObservedVersions } | undefined;
};
