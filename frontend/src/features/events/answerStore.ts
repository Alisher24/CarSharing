import { afterSuccess, type Resource } from '../../shared/api/Resource.ts';
import type { ReadCoordinator, ReadTicket } from './coordinator.ts';

/**
 * storedAnswer turns one answer into the resource to hold, or into nothing when the answer must be
 * ignored. An answer that belongs to an ended session, or to a read the coordinator has already
 * answered, is dropped; so is an answer that publishes nothing the screen does not already show.
 *
 * It is the one place a read's answer becomes what is on screen, so a snapshot and a feed of pages
 * are stored under the same rules rather than under two sets of them.
 */
export function storedAnswer<T>(
  coordinator: ReadCoordinator | undefined,
  ticket: ReadTicket | undefined,
  session: string | undefined,
  value: unknown,
): Resource<T> | undefined {
  if (coordinator === undefined || ticket === undefined || session === undefined) return afterSuccess(value as T);
  if (!coordinator.accepts(ticket)) return undefined;

  const handlers = coordinator.answer(ticket.document);
  if (!handlers.accepts(session)) return undefined;

  const stored = handlers.observe(value, session);
  if (stored === undefined) return undefined;

  coordinator.cover(ticket.document, stored.covered);
  coordinator.stored(ticket);
  return afterSuccess(stored.value as T);
}
