import type { GeoJSONSource, Map } from 'maplibre-gl';
import type { Zone } from '../../shared/api/catalog';
import { geometryBounds } from './geometryBounds';

/** What the published boundaries are drawn from and in, so a redraw replaces them rather than adds. */
const ZONE_SOURCE = 'service-zones';
const ZONE_FILL_LAYER = 'service-zones-fill';
const ZONE_OUTLINE_LAYER = 'service-zones-outline';

/**
 * How a boundary is drawn: a faint fill under a dashed edge, so a vehicle standing near it stays
 * readable through it. The colour is stated once because the fill and the edge are the same boundary
 * seen twice, and a second literal is how the two come to be different greens.
 */
const ZONE_COLOUR = '#518b72';
const ZONE_FILL = { 'fill-color': ZONE_COLOUR, 'fill-opacity': 0.07 };
const ZONE_OUTLINE = { 'line-color': ZONE_COLOUR, 'line-width': 2, 'line-dasharray': [6, 5] };

/**
 * putServiceZones draws the operator boundaries the service published, exactly as it published them.
 * A boundary that could not be read draws nothing at all: an invented one would be a claim about
 * where a rental may end. The source and its two layers are added once, so a redraw replaces the
 * boundaries rather than stacking a second copy of them under the first.
 */
export function putServiceZones(map: Map, zones: readonly Zone[]): void {
  const published = zoneDocument(zones);
  const existing = map.getSource(ZONE_SOURCE) as GeoJSONSource | undefined;
  if (existing !== undefined) {
    void existing.setData(published);
    return;
  }

  map.addSource(ZONE_SOURCE, { type: 'geojson', data: published });
  map.addLayer({ id: ZONE_FILL_LAYER, type: 'fill', source: ZONE_SOURCE, paint: ZONE_FILL });
  map.addLayer({ id: ZONE_OUTLINE_LAYER, type: 'line', source: ZONE_SOURCE, paint: ZONE_OUTLINE });
}

/** The box the published boundaries span, or nothing when the service published none. */
export function serviceZoneBounds(zones: readonly Zone[]): ReturnType<typeof geometryBounds> {
  return geometryBounds(zones.map((zone) => zone.geometry));
}

/** The boundaries as one document, which is how a single source carries them all. */
function zoneDocument(zones: readonly Zone[]) {
  return {
    type: 'FeatureCollection' as const,
    features: zones.map((zone) => ({
      type: 'Feature' as const,
      id: zone.id,
      properties: { name: zone.name },
      geometry: zone.geometry,
    })),
  };
}
