import { CircleMarker, LayerGroup, type CircleMarkerOptions } from 'leaflet';
import type { Vehicle } from '../../shared/api/catalog';
import { POWERTRAIN_LABELS, statusText } from '../fleet/fleetCopy';
import { toLatLng } from './coordinates';
import {
  MARKER_FILL_OPACITY,
  MARKER_OUTLINE_WEIGHT,
  MARKER_RADIUS,
  SELECTED_MARKER_OUTLINE_WEIGHT,
  SELECTED_MARKER_RADIUS,
  selectedOutlineColour,
  statusColour,
} from './markerStyle';

/** What the marker layer is showing right now, so a redraw changes only what differs. */
export type MarkerSelection = { vehicles: readonly Vehicle[]; selectedId: string | undefined };

/**
 * syncVehicleMarkers redraws the markers from one filtered result, which is the same result the
 * list is built from. A vehicle that leaves that result leaves the map with it.
 */
export function syncVehicleMarkers(
  layer: LayerGroup,
  selection: MarkerSelection,
  onSelect: (vehicleId: string) => void,
): void {
  layer.clearLayers();
  for (const vehicle of selection.vehicles) {
    layer.addLayer(vehicleMarker(vehicle, vehicle.id === selection.selectedId, onSelect));
  }
}

function vehicleMarker(vehicle: Vehicle, selected: boolean, onSelect: (vehicleId: string) => void): CircleMarker {
  const marker = new CircleMarker(toLatLng(vehicle.position.coordinates), markerStyle(vehicle, selected));
  marker.bindTooltip(`${vehicle.model} · ${POWERTRAIN_LABELS[vehicle.powertrain_type]} · ${statusText(vehicle)}`);
  marker.on('click', () => onSelect(vehicle.id));
  return marker;
}

/** The vehicle a person has selected is drawn larger and outlined in ink, so it stands out. */
function markerStyle(vehicle: Vehicle, selected: boolean): CircleMarkerOptions {
  const colour = statusColour(vehicle.status);
  if (!selected) {
    return {
      radius: MARKER_RADIUS,
      color: colour,
      weight: MARKER_OUTLINE_WEIGHT,
      fillColor: colour,
      fillOpacity: MARKER_FILL_OPACITY,
    };
  }
  return {
    radius: SELECTED_MARKER_RADIUS,
    color: selectedOutlineColour(),
    weight: SELECTED_MARKER_OUTLINE_WEIGHT,
    fillColor: colour,
    fillOpacity: MARKER_FILL_OPACITY,
  };
}
