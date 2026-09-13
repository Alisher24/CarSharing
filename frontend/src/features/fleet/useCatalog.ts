import { useEffect, useMemo, useRef, useState } from 'react';
import {
  fetchFleet,
  fetchServiceZones,
  fetchTariffs,
  type FleetSnapshot,
  type Tariff,
  type Vehicle,
  type Zone,
} from '../../shared/api/catalog';
import { loadedValue, type Resource } from '../../shared/api/Resource';
import { useResource, type ResourceHandle, type ResourceRead } from '../../shared/api/useResource';
import { catalogAnswers } from '../events/catalogAnswers';
import { changedResources } from '../events/changes';
import { createReadCoordinator, PUBLIC_DOCUMENT_KINDS, type ReadCoordinator } from '../events/coordinator';
import { RECONCILE_MILLISECONDS, reconciliationEnabled } from '../events/reconciliation';
import type { DocumentKind } from '../events/readCycle';
import type { EventsFeed } from '../events/useEvents';
import type { FleetReading } from '../events/fleetSnapshot';

/**
 * How long a batch of changes is collected before it is read. One window costs one read of each
 * resource it named, so a burst of changes to the fleet is one catalog read rather than one per
 * vehicle.
 */
const COALESCE_MILLISECONDS = 100;

/** The catalog is public, so its reads belong to no session; their sequencing still belongs to one. */
const ANONYMOUS_SESSION = 'anonymous';

/** A reading nothing has been read into yet: no moment, because no snapshot was computed at one. */
const NOTHING_READ: FleetReading = { vehicles: [], serverTime: '' };

/**
 * Catalog is the three public resources, each loaded on its own. One of them failing leaves the
 * other two on screen, and each is retried by itself.
 */
export type Catalog = {
  fleet: ResourceHandle<FleetSnapshot>;
  zones: ResourceHandle<Zone[]>;
  tariffs: ResourceHandle<Tariff[]>;
};

/**
 * useCatalog reads the public resources and keeps them up to date from the change stream and from
 * its own reconciliation. A change only decides when to read; what is read is always the whole
 * resource, so a signal that never arrived costs a delay rather than a wrong screen.
 */
export function useCatalog(events: EventsFeed): Catalog {
  const fleetReading = useRef<FleetReading>(NOTHING_READ);
  const coordinator = useState(() =>
    createReadCoordinator(ANONYMOUS_SESSION, catalogAnswers({ session: ANONYMOUS_SESSION, fleetReading })),
  )[0];
  const reads = useMemo(() => readsOf(coordinator), [coordinator]);

  const fleet = useResource(fetchFleet, reads.vehicles);
  const zones = useResource(fetchServiceZones, reads.zones);
  const tariffs = useResource(fetchTariffs, reads.tariffs);

  useCoalescedChanges(coordinator, events.changes);
  useReadyFrame(coordinator, events.readyCount);
  useReconciliation(coordinator);

  return { fleet, zones, tariffs };
}

/**
 * One resource's part in the cycle: which document it is, and whose session it is read in. The
 * parts are held for as long as the coordinator is, because a resource watches the part for
 * changes and a rebuilt part would restart every read on every render.
 */
function readsOf(coordinator: ReadCoordinator): Record<DocumentKind, ResourceRead> {
  return {
    vehicles: { coordinator, document: 'vehicles', session: ANONYMOUS_SESSION },
    zones: { coordinator, document: 'zones', session: ANONYMOUS_SESSION },
    tariffs: { coordinator, document: 'tariffs', session: ANONYMOUS_SESSION },
    // The private resource is read by the reader of the account's own reservation; this coordinator
    // answers nothing for it, and nothing here ever asks for it.
    current: { coordinator, document: 'current', session: ANONYMOUS_SESSION },
  };
}

/**
 * Feeds the changes one connection delivered into the cycle. Each change is remembered at once, so
 * one that arrives during a request is not lost, and the reads they ask for are opened together
 * once the window has passed.
 */
function useCoalescedChanges(coordinator: ReadCoordinator, changes: EventsFeed['changes']): void {
  useEffect(() => {
    for (const change of changes) coordinator.observe(change.resource, change.id, change.version);

    const batch = window.setTimeout(() => askFor(coordinator, changedResources(changes)), COALESCE_MILLISECONDS);
    return () => window.clearTimeout(batch);
  }, [coordinator, changes]);
}

/**
 * Asks for a read of each resource a window named. The resource itself decides whether anything is
 * to read, so a signal that an answer has already covered costs no request.
 */
function askFor(coordinator: ReadCoordinator, documents: readonly DocumentKind[]): void {
  for (const document of documents) {
    if (coordinator.due(document)) coordinator.force(document);
  }
}

/** Answers a handshake by asking for every resource to be read again, reconnects included. */
function useReadyFrame(coordinator: ReadCoordinator, readyCount: number): void {
  useEffect(() => {
    if (readyCount > 0) coordinator.request(PUBLIC_DOCUMENT_KINDS);
  }, [coordinator, readyCount]);
}

/**
 * Asks for every resource to be read again on a fixed interval. This is the one request that does
 * not depend on anything having changed, because its whole purpose is to find the changes nothing
 * told this client about — a signal the stream lost, or a stream that was down while it happened.
 */
function useReconciliation(coordinator: ReadCoordinator): void {
  useEffect(() => {
    const repeat = window.setInterval(() => {
      if (reconciliationEnabled()) coordinator.request(PUBLIC_DOCUMENT_KINDS);
    }, RECONCILE_MILLISECONDS);
    return () => window.clearInterval(repeat);
  }, [coordinator]);
}

/** The vehicles last read, or none at all before the first reading arrives. */
export function catalogVehicles(catalog: Catalog): readonly Vehicle[] {
  return loadedValue(catalog.fleet.resource)?.vehicles ?? [];
}

/**
 * What a view found when it looked for the value it needs. The three ways of having nothing are
 * kept apart, because they are three different things to tell a person: the answer has not arrived
 * yet, the service answered and published none, or nothing could be read at all.
 */
export type Found<T> =
  { state: 'loading' } | { state: 'found'; value: T } | { state: 'none' } | { state: 'unreachable' };

function found<T>(resource: Resource<unknown>, value: T | undefined): Found<T> {
  if (value !== undefined) return { state: 'found', value };
  if (resource.phase === 'loading') return { state: 'loading' };
  if (resource.phase === 'failed') return { state: 'unreachable' };
  return { state: 'none' };
}

/** What the map found when it looked for the service areas to draw. */
export function foundZones(catalog: Catalog): Found<readonly Zone[]> {
  const areas = loadedValue(catalog.zones.resource);
  return found(catalog.zones.resource, areas !== undefined && areas.length > 0 ? areas : undefined);
}

/**
 * What a card found when it looked for the price list to quote. The operator publishes one
 * demonstration tariff; nothing here substitutes a price of zero for a tariff it could not read.
 */
export function foundTariff(catalog: Catalog): Found<Tariff> {
  return found(catalog.tariffs.resource, loadedValue(catalog.tariffs.resource)?.[0]);
}
