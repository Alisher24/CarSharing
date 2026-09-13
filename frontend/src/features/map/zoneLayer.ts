import { GeoJSON, LatLngBounds, type LayerGroup } from 'leaflet';
import type { Zone } from '../../shared/api/catalog';

const ZONE_STYLE = { color: '#518b72', weight: 2, dashArray: '6 5', fillColor: '#518b72', fillOpacity: 0.07 };

/**
 * drawServiceZones puts the operator boundaries on the map exactly as the service published them.
 * A zone that could not be read draws nothing at all: an invented boundary would be a claim about
 * where a rental may end.
 */
export function drawServiceZones(layer: LayerGroup, zones: readonly Zone[]): LatLngBounds | undefined {
  layer.clearLayers();
  let covered: LatLngBounds | undefined;
  for (const zone of zones) {
    const drawn = new GeoJSON(zone.geometry, { style: ZONE_STYLE, interactive: false });
    layer.addLayer(drawn);
    covered = covered ? covered.extend(drawn.getBounds()) : drawn.getBounds();
  }
  return covered;
}
