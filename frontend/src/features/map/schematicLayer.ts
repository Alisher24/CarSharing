import { CircleMarker, LayerGroup, Polyline, type Map as LeafletMap } from 'leaflet';
import { SCHEMATIC_LANDMARKS, SCHEMATIC_STREETS, type SchematicStreet } from './bishkekSchematic';
import { toLatLng, toLatLngs } from './coordinates';

/** How the scheme is drawn. Avenues carry more weight so the shape of the city is readable. */
const STREET_WIDTHS: Record<SchematicStreet['rank'], number> = { avenue: 5, street: 3 };
const STREET_COLOUR = '#c9d3c2';
const LANDMARK_COLOUR = '#8fa08a';
const LANDMARK_RADIUS = 4;

/**
 * drawSchematic puts the local scheme of Bishkek under everything else on the map. It draws from
 * coordinates the application ships, so the map has a recognisable background with no tile
 * provider, no external request and no key.
 */
export function drawSchematic(map: LeafletMap): LayerGroup {
  const schematic = new LayerGroup();
  for (const street of SCHEMATIC_STREETS) {
    schematic.addLayer(drawStreet(street));
  }
  for (const landmark of SCHEMATIC_LANDMARKS) {
    schematic.addLayer(drawLandmark(landmark.name, landmark.at));
  }
  schematic.addTo(map);
  return schematic;
}

function drawStreet(street: SchematicStreet): Polyline {
  return new Polyline(toLatLngs(street.path), {
    color: STREET_COLOUR,
    weight: STREET_WIDTHS[street.rank],
    interactive: false,
  }).bindTooltip(street.name, { direction: 'center', className: 'map-street-name' });
}

function drawLandmark(name: string, at: Parameters<typeof toLatLng>[0]): CircleMarker {
  return new CircleMarker(toLatLng(at), {
    radius: LANDMARK_RADIUS,
    color: LANDMARK_COLOUR,
    weight: 1,
    fillColor: LANDMARK_COLOUR,
    fillOpacity: 1,
    interactive: false,
  }).bindTooltip(name, { direction: 'right', permanent: true, className: 'map-landmark-name' });
}
