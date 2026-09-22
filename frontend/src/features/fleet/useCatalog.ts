import { useMemo, useRef, useState } from 'react';
import {
  fetchFleet,
  fetchServiceZones,
  fetchTariffs,
  type FleetSnapshot,
  type Tariff,
  type Vehicle,
  type Zone,
} from '../../shared/api/catalog.ts';
import { loadedValue } from '../../shared/read/Resource.ts';
import { createReadCoordinator, PUBLIC_DOCUMENT_KINDS, type ReadCoordinator } from '../../shared/read/coordinator.ts';
import { found, type Found } from '../../shared/read/presence.ts';
import type { PublicDocumentKind } from '../../shared/read/readCycle.ts';
import { useReadCycle } from '../../shared/read/useReadCycle.ts';
import { useResource, type ResourceHandle, type ResourceRead } from '../../shared/read/useResource.ts';
import type { CycleFeed } from '../../shared/read/useReadCycle.ts';
import { catalogAnswers } from './catalogAnswers.ts';
import type { FleetReading } from './fleetSnapshot.ts';

/** The catalog is public, so its reads belong to no session; their sequencing still belongs to one. */
const ANONYMOUS_SESSION = 'anonymous';

/** A reading nothing has been read into yet: no moment, because no snapshot was computed at one. */
const NOTHING_READ: FleetReading = { vehicles: [], serverTime: '' };

/** Catalog is the three public resources, each loaded on its own. */
export type Catalog = {
  fleet: ResourceHandle<FleetSnapshot>;
  zones: ResourceHandle<Zone[]>;
  tariffs: ResourceHandle<Tariff[]>;
};

/**
 * useCatalog reads the public resources and keeps them up to date from the change stream and from
 * its own reconciliation, which is the one read cycle every reader of the application takes part in.
 * A change only decides when to read; what is read is always the whole resource, so a signal that
 * never arrived costs a delay rather than a wrong screen.
 *
 * One of the three failing leaves the other two on screen, and each is retried by itself.
 */
export function useCatalog(events: CycleFeed): Catalog {
  const fleetReading = useRef<FleetReading>(NOTHING_READ);
  const coordinator = useState(() =>
    createReadCoordinator(ANONYMOUS_SESSION, catalogAnswers({ session: ANONYMOUS_SESSION, fleetReading })),
  )[0];
  const reads = useMemo(() => readsOf(coordinator), [coordinator]);

  const fleet = useResource(fetchFleet, reads.vehicles);
  const zones = useResource(fetchServiceZones, reads.zones);
  const tariffs = useResource(fetchTariffs, reads.tariffs);

  useReadCycle(coordinator, events, PUBLIC_DOCUMENT_KINDS);

  return { fleet, zones, tariffs };
}

/**
 * One resource's part in the cycle: which document it is, and whose session it is read in. The
 * parts are held for as long as the coordinator is, because a resource watches the part for
 * changes and a rebuilt part would restart every read on every render.
 */
function readsOf(coordinator: ReadCoordinator): Record<PublicDocumentKind, ResourceRead> {
  return {
    vehicles: { coordinator, document: 'vehicles', session: ANONYMOUS_SESSION },
    zones: { coordinator, document: 'zones', session: ANONYMOUS_SESSION },
    tariffs: { coordinator, document: 'tariffs', session: ANONYMOUS_SESSION },
  };
}

/** The vehicles last read, or none at all before the first reading arrives. */
export function catalogVehicles(catalog: Catalog): readonly Vehicle[] {
  return loadedValue(catalog.fleet.resource)?.vehicles ?? [];
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
