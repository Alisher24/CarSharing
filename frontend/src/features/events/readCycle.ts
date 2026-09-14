import type { ObservedVersions } from './changes.ts';

/**
 * One resource a reader keeps up to date, which is what a change signal and a ready frame address.
 * The catalog reads the public ones; the reservation of the signed-in person and the notifications
 * addressed to them are private, and each is read by its own reader. A coordinator states what it
 * does with every document, so one that belongs to another reader is named rather than left out.
 */
export type DocumentKind = 'vehicles' | 'zones' | 'tariffs' | 'current' | 'notifications';

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
