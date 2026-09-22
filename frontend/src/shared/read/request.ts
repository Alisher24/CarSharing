import type { ReadCoordinator } from './coordinator.ts';
import type { DocumentKind } from './readCycle.ts';

/**
 * Asks for a read of each document a window named. The document itself decides whether anything is
 * to read, so a signal an answer has already covered costs no request.
 */
export function requestDocuments(coordinator: ReadCoordinator, documents: readonly DocumentKind[]): void {
  for (const document of documents) {
    if (coordinator.due(document)) coordinator.force(document);
  }
}
