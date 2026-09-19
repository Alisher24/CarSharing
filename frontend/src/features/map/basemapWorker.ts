import { setWorkerUrl } from 'maplibre-gl';
// The worker is a module of the library rather than a file it names, so a bundler has to be told to
// build it: imported as a URL, it is bundled with the library's own chunks the way the library itself
// bundles them. Copied as a plain asset instead, it cannot import the chunk it shares with the
// library, fails to start, and every tile stays in the state it was asked for and never arrives —
// while the style, the source and the archive all look healthy.
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url';

/**
 * The one place the map's worker is located. It is stated before the first map is created, because the
 * library reads it once and a map made first would keep looking in the wrong place.
 */
export function installBasemapWorker(): void {
  setWorkerUrl(workerUrl);
}
