import type { NotificationCollection } from '../../shared/api/notifications.ts';
import type { AnswerHandlers, DocumentKind } from '../events/readCycle.ts';
import { versionsOf } from '../events/observedResource.ts';
import type { NotificationReading } from './notificationReading.ts';
import { updatesCollection } from './notificationReading.ts';

/** What one session of reading the account's own notifications knows, which one answer needs to store. */
export type NotificationSession = {
  session: string;
  reading: { collection: NotificationCollection | undefined };
};

/**
 * notificationAnswers builds the answers of the coordinator that reads the notifications addressed
 * to one account. An answer is stored only when it belongs to that account and was computed later
 * than the one already held, and it covers every version it publishes, so the changes it answers for
 * are the ones the cycle stops asking about.
 *
 * The reservation and the public resources are answered by nobody here: a signal about one of them
 * belongs to the reader that holds it, and this one reads only the collection.
 */
export function notificationAnswers(state: NotificationSession): Record<DocumentKind, AnswerHandlers> {
  return {
    vehicles: notReadHere,
    zones: notReadHere,
    tariffs: notReadHere,
    current: notReadHere,
    notifications: collectionAnswer(state),
  };
}

// A document this coordinator does not read answers nothing, so a signal that names one is left to
// the reader that holds it.
const notReadHere: AnswerHandlers = {
  accepts: () => false,
  observe: () => undefined,
};

/**
 * The answer of the collection. An answer of another account is refused before it is looked at, and
 * one whose value carries no collection — a reader with no session asks for nothing — stores nothing
 * rather than an empty page it would be wrong to show.
 */
function collectionAnswer(state: NotificationSession): AnswerHandlers {
  return {
    accepts: (session) => session === state.session,
    observe: (value, session) => {
      if (session !== state.session) return undefined;

      const answer = value as NotificationReading | undefined;
      if (answer === undefined) return undefined;
      if (!updatesCollection(state.reading.collection, answer.collection)) return undefined;

      state.reading.collection = answer.collection;
      return { value: answer, covered: versionsOf(answer.collection.items) };
    },
  };
}
