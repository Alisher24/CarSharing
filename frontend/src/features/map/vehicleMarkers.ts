import { Marker, type Map as MapLibreMap } from 'maplibre-gl';
import type { Vehicle } from '../../shared/api/catalog.ts';
import { POWERTRAIN_LABELS, statusText } from '../../shared/vehicle/spell.ts';
import { statusColourProperty } from './markerStyle.ts';

/** What the marker layer is showing right now, so a redraw changes only what differs. */
export type MarkerSelection = { vehicles: readonly Vehicle[]; selectedId: string | undefined };

/** One vehicle on the map, with the element a person clicks and a check finds. */
export type VehicleMarker = { marker: Marker; element: HTMLElement };

/**
 * syncVehicleMarkers draws one marker per vehicle of the current result, which is the same result the
 * list is built from: a vehicle that leaves that result leaves the map with it.
 *
 * The markers are elements rather than a circle layer of the map's own drawing. A circle layer is
 * painted by the map into its own canvas, which nothing can click, read or check; an element is
 * something a person clicks and a check finds, and twenty-five of them cost nothing worth measuring.
 */
export function syncVehicleMarkers(
  map: MapLibreMap,
  shown: Map<string, VehicleMarker>,
  selection: MarkerSelection,
  onSelect: (vehicleId: string) => void,
): void {
  const wanted = new Set(selection.vehicles.map((vehicle) => vehicle.id));
  for (const [id, held] of shown) {
    if (!wanted.has(id)) {
      held.marker.remove();
      shown.delete(id);
    }
  }

  for (const vehicle of selection.vehicles) {
    const held = shown.get(vehicle.id) ?? heldFor(vehicle, shown, onSelect);
    held.marker.setLngLat(vehicle.position.coordinates).addTo(map);
    held.element.dataset.selected = String(vehicle.id === selection.selectedId);
    held.element.style.background = `var(${statusColourProperty(vehicle.status)})`;
    held.element.title = markerLabel(vehicle);
    held.element.setAttribute('aria-label', held.element.title);
  }
}

/**
 * The marker kept for one vehicle. A marker is made once and moved afterwards: the model publishes a
 * new position for every vehicle every few seconds, and a map that discarded and rebuilt twenty-five
 * elements that often would lose whatever the browser holds for them, a click in progress among it.
 */
function heldFor(
  vehicle: Vehicle,
  shown: Map<string, VehicleMarker>,
  onSelect: (vehicleId: string) => void,
): VehicleMarker {
  const element = document.createElement('button');
  element.type = 'button';
  element.className = 'map-vehicle-marker';
  element.addEventListener('click', () => onSelect(vehicle.id));

  const held = { marker: new Marker({ element, anchor: 'center' }), element };
  shown.set(vehicle.id, held);
  return held;
}

/** What a person reads about one vehicle without opening it, which its state is part of. */
function markerLabel(vehicle: Vehicle): string {
  return `${vehicle.model} · ${POWERTRAIN_LABELS[vehicle.powertrain_type]} · ${statusText(vehicle)}`;
}
