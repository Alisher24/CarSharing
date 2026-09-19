import { addProtocol } from 'maplibre-gl';
import { Protocol } from 'pmtiles';

/**
 * The one registration of the archive protocol, and the one place a map hears that the archive could
 * not be read. MapLibre keeps its protocols in a single global table, so a second registration of the
 * same scheme replaces the first, and a component that registered on every mount would leave one map
 * reading an archive another had opened.
 */

/** The scheme the archive is read through, which is what the style says its source is. */
const ARCHIVE_SCHEME = 'pmtiles';

/** What a map is told when a request for the archive fails. */
export type ArchiveFailure = (failure: unknown) => void;

/** The maps listening for a failure of the archive, in the order they began listening. */
const listeners = new Set<ArchiveFailure>();

/** Whether a request for the archive has already failed. */
let failed = false;

/** Whether the scheme has already been put in MapLibre's table. */
let installed = false;

/**
 * Teaches MapLibre to read the archive. It is called once, before the first map is rendered: the style
 * names its source through this scheme, and a map created before the protocol exists cannot load one.
 *
 * A request the protocol cannot answer is reported to every map listening, and the failure then
 * carries on to MapLibre as it would have anyway. That is what lets the pane say the basemap is
 * missing rather than stay empty for a reason nobody is told.
 */
export function installBasemapProtocol(): void {
  if (installed) return;
  installed = true;

  const protocol = new Protocol();
  addProtocol(ARCHIVE_SCHEME, async (request, controller) => {
    try {
      return await protocol.tile(request, controller);
    } catch (failure) {
      failed = true;
      for (const listener of listeners) listener(failure);
      throw failure;
    }
  });
}

/**
 * Listen for the archive failing to be read.
 *
 * @param {ArchiveFailure} failure what to tell, told at once when one has already happened
 * @returns {() => void} how to stop listening
 */
export function onArchiveFailure(failure: ArchiveFailure): () => void {
  if (failed) failure(new Error('the request for the archive failed before this map was drawn'));
  listeners.add(failure);
  return () => {
    listeners.delete(failure);
  };
}
