import { LatLngBounds, LayerGroup, Map as LeafletMap } from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { useCallback, useEffect, useRef, useState, type RefObject } from 'react';
import type { Vehicle, Zone } from '../../shared/api/catalog';
import { ResourceNotice, type AbsenceCopy } from '../fleet/ResourceNotice';
import type { Found } from '../fleet/useCatalog';
import { SCHEMATIC_CAPTION } from './bishkekSchematic';
import { toLatLng } from './coordinates';
import { MapLegend } from './MapLegend';
import { drawSchematic } from './schematicLayer';
import { syncVehicleMarkers } from './vehicleMarkers';
import { drawServiceZones } from './zoneLayer';

/** How closely the map follows a vehicle a person has selected. */
const SELECTED_VEHICLE_ZOOM = 16;

/** Room left around the service zone when the map is returned to the whole area. */
const ZONE_PADDING: [number, number] = [24, 24];

/** Where the map opens before a zone has loaded: the demonstration area of Bishkek. */
const FALLBACK_CENTRE: [number, number] = [42.87, 74.6];
const FALLBACK_ZOOM = 12;

/**
 * The map is drawn from vector coordinates rather than tiles, so it is not tied to whole zoom
 * levels. Letting a fit land between them is what makes the service area actually fill the pane
 * instead of stopping at the nearest level that happens to contain it.
 */
const CONTINUOUS_ZOOM = 0;

/**
 * What the map says when there is no boundary to draw. It never draws one of its own: a made-up
 * boundary would be a claim about where a rental may end.
 */
const ZONE_ABSENCE: AbsenceCopy = {
  loading: 'Загружаем границу зоны…',
  none: 'Зона обслуживания не задана',
  unreachable: 'Граница зоны не загружена',
};

type FleetMapProps = {
  vehicles: readonly Vehicle[];
  zones: Found<readonly Zone[]>;
  onRetryZones: () => void;
  selectedId: string | undefined;
  onSelect: (vehicleId: string) => void;
};

/**
 * FleetMap is the main screen. It draws the local scheme of Bishkek, the service zone and one
 * marker per vehicle of the current result, and it makes no request of its own: everything it
 * shows is passed to it, so a resource that failed to load is explained rather than invented.
 */
export function FleetMap({ vehicles, zones, onRetryZones, selectedId, onSelect }: FleetMapProps) {
  const container = useRef<HTMLDivElement>(null);
  const map = useRef<LeafletMap>(null);
  const zoneLayer = useRef<LayerGroup>(null);
  const markerLayer = useRef<LayerGroup>(null);
  const [zoneBounds, setZoneBounds] = useState<LatLngBounds>();
  const drawn = zones.state === 'found' ? zones.value : EMPTY_ZONES;

  useEffect(() => {
    if (container.current === null) return undefined;
    const created = new LeafletMap(container.current, {
      center: FALLBACK_CENTRE,
      zoom: FALLBACK_ZOOM,
      zoomSnap: CONTINUOUS_ZOOM,
      // The caption below names the scheme and the library it is drawn with, so the map needs no
      // control of its own for it.
      attributionControl: false,
    });
    drawSchematic(created);
    zoneLayer.current = new LayerGroup().addTo(created);
    markerLayer.current = new LayerGroup().addTo(created);
    map.current = created;
    return () => {
      created.remove();
      map.current = null;
    };
  }, []);

  // Leaflet caches the size of its viewport, so a pane that is laid out or resized after the map
  // was created has to say so; otherwise every later fit is computed against the wrong rectangle.
  useEffect(() => {
    if (container.current === null) return undefined;
    const resized = new ResizeObserver(() => map.current?.invalidateSize());
    resized.observe(container.current);
    return () => resized.disconnect();
  }, []);

  const showWholeZone = useCallback(() => {
    if (map.current === null || zoneBounds === undefined) return;
    map.current.invalidateSize();
    map.current.fitBounds(zoneBounds, { padding: ZONE_PADDING });
  }, [zoneBounds]);

  useEffect(() => {
    if (zoneLayer.current === null) return;
    setZoneBounds(drawServiceZones(zoneLayer.current, drawn));
  }, [drawn]);

  // The first zone to arrive decides the opening view, which is the whole service area.
  const framed = useRef(false);
  useEffect(() => {
    if (framed.current || zoneBounds === undefined) return;
    framed.current = true;
    showWholeZone();
  }, [zoneBounds, showWholeZone]);

  useEffect(() => {
    if (markerLayer.current === null) return;
    syncVehicleMarkers(markerLayer.current, { vehicles, selectedId }, onSelect);
  }, [vehicles, selectedId, onSelect]);

  useSelectedVehicleView(map, vehicles, selectedId);

  return (
    <div className="map">
      <div className="map-canvas" ref={container} role="application" aria-label="Карта парка" />
      <div className="map-caption">{SCHEMATIC_CAPTION}</div>
      <div className="map-controls">
        <button className="map-action" type="button" onClick={showWholeZone} disabled={zoneBounds === undefined}>
          Показать зону
        </button>
        <ResourceNotice found={zones} copy={ZONE_ABSENCE} onRetry={onRetryZones} />
      </div>
      <MapLegend zones={drawn} />
    </div>
  );
}

/** One empty list, so a render with no zone to draw does not look like a changed one. */
const EMPTY_ZONES: readonly Zone[] = [];

/**
 * useSelectedVehicleView brings a newly selected vehicle into view. It moves the map only when the
 * selection itself changes, so a person who has panned away or asked for the whole zone keeps the
 * view they chose while the same vehicle stays selected.
 */
function useSelectedVehicleView(
  map: RefObject<LeafletMap | null>,
  vehicles: readonly Vehicle[],
  selectedId: string | undefined,
): void {
  const shown = useRef<string>(undefined);
  useEffect(() => {
    if (selectedId === undefined) {
      shown.current = undefined;
      return;
    }
    if (shown.current === selectedId) return;
    const selected = vehicles.find((vehicle) => vehicle.id === selectedId);
    if (map.current === null || selected === undefined) return;
    shown.current = selectedId;
    map.current.flyTo(toLatLng(selected.position.coordinates), SELECTED_VEHICLE_ZOOM);
  }, [map, vehicles, selectedId]);
}
