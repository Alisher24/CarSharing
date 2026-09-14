import type { FleetSnapshot, Tariff, Zone } from '../../shared/api/catalog.ts';
import type { AnswerHandlers, DocumentKind } from './readCycle.ts';
import { mergeFleetSnapshot, updatesFleetReading, type FleetReading } from './fleetSnapshot.ts';
import { versionsOf, type Versioned } from './observedResource.ts';

/** What one session of reading the public catalog knows, which one answer needs to be stored. */
export type CatalogSession = {
  session: string;
  fleetReading: { current: FleetReading };
};

/**
 * catalogAnswers builds the answers of the catalog's coordinator. Each one states which session it
 * belongs to and what storing it means; how many answers may still arrive is the coordinator's
 * business. The private documents are answered by nobody here: the catalog reads what a visitor
 * reads, and the reservation of the signed-in person and the notifications addressed to them have
 * their own readers.
 */
export function catalogAnswers(catalog: CatalogSession): Record<DocumentKind, AnswerHandlers> {
  return {
    vehicles: fleetAnswers(catalog),
    zones: listAnswers<Zone>(catalog.session),
    tariffs: listAnswers<Tariff>(catalog.session),
    current: notReadHere,
    notifications: notReadHere,
  };
}

// A document this coordinator does not read answers nothing, so a signal that names one is left to
// the reader that holds it.
const notReadHere: AnswerHandlers = {
  accepts: () => false,
  observe: () => undefined,
};

// The fleet is the one resource whose answer can say nothing new: every vehicle in it can be a
// version the snapshot already holds, and freshness is measured against the moment the read was
// computed at rather than against the moment it arrived.
function fleetAnswers(catalog: CatalogSession): AnswerHandlers {
  return {
    accepts: (session) => session === catalog.session,
    observe: (value, session) => {
      if (session !== catalog.session) return undefined;

      const snapshot = value as FleetSnapshot;
      const worthShowing = updatesFleetReading(catalog.fleetReading.current, snapshot);
      const merged = mergeFleetSnapshot(catalog.fleetReading.current, snapshot);
      catalog.fleetReading.current = merged;

      if (!worthShowing) return undefined;
      return { value: merged, covered: versionsOf(snapshot.vehicles) };
    },
  };
}

// A zone and a tariff collection carries no moment of its own, so its answer is stored as it is
// and covers every version it publishes.
function listAnswers<T extends Versioned>(session: string): AnswerHandlers {
  return {
    accepts: (answering) => answering === session,
    observe: (value, answering) => {
      if (answering !== session) return undefined;

      return { value, covered: versionsOf(value as readonly T[]) };
    },
  };
}
