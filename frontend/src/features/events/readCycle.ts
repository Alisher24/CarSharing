import type { ObservedVersions } from './changes.ts';

/** A resource every visitor may read, which the public catalog keeps up to date. */
export type PublicDocumentKind = 'vehicles' | 'zones' | 'tariffs';

/**
 * A resource that belongs to one account. It is the address a private change signal carries rather
 * than the name of a screen: the rentals of an account are one address, read both by the panel that
 * shows the rental in force and by the feed that lists the ones that have finished.
 */
export type PrivateDocumentKind = 'rentals' | 'notifications' | 'invoices';

/**
 * One resource a reader keeps up to date, which is what a change signal and a ready frame address.
 * The catalog reads the public ones; each private one is read by the readers that hold it, and a
 * coordinator states what it does with every document, so one that belongs to another reader is
 * named rather than left out.
 */
export type DocumentKind = PublicDocumentKind | PrivateDocumentKind;

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
