import type { ObservedVersions } from './changes.ts';

/** One public resource the catalog reads, which is what a change signal and a ready frame address. */
export type DocumentKind = 'vehicles' | 'zones' | 'tariffs';

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
