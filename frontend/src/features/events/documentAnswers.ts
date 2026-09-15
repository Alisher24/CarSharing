import type { AnswerHandlers, DocumentKind } from './readCycle.ts';

/**
 * A document this coordinator does not read answers nothing, so a signal that names one is left to
 * the reader that holds it.
 */
const NOT_READ_HERE: AnswerHandlers = {
  accepts: () => false,
  observe: () => undefined,
};

/**
 * Every document answered by nobody, which is what a coordinator that names none of them would do.
 * It is the one place the set of documents is enumerated for this purpose: a reader states the
 * documents it holds and nothing else, and a new document joins every coordinator here rather than
 * in a list each of them keeps.
 */
const NOTHING_READ_HERE: Record<DocumentKind, AnswerHandlers> = {
  vehicles: NOT_READ_HERE,
  zones: NOT_READ_HERE,
  tariffs: NOT_READ_HERE,
  rentals: NOT_READ_HERE,
  notifications: NOT_READ_HERE,
  invoices: NOT_READ_HERE,
};

/**
 * answersOf builds the answers of a coordinator that reads only the documents it names. A signal
 * about any other document is answered by nobody here, which is what leaves it to the reader that
 * holds it.
 */
export function answersOf(named: Partial<Record<DocumentKind, AnswerHandlers>>): Record<DocumentKind, AnswerHandlers> {
  return { ...NOTHING_READ_HERE, ...named };
}
