import { LngLatBounds, Map as MapLibreMap } from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import { useCallback, useEffect, useRef, useState, type RefObject } from 'react';
import type { Vehicle, Zone } from '../../shared/api/catalog';
import { ResourceNotice, type AbsenceCopy } from '../fleet/ResourceNotice';
import type { Found } from '../fleet/useCatalog';
import { BASEMAP_CAPTION, BASEMAP_CENTRE, basemapStyle } from './basemap';
import { onArchiveFailure } from './basemapProtocol';
import { MapLegend } from './MapLegend';
import { syncVehicleMarkers, type VehicleMarker } from './vehicleMarkers';
import { putServiceZones, serviceZoneBounds } from './zoneLayer';

/** How closely the map follows a vehicle a person has selected. */
const SELECTED_VEHICLE_ZOOM = 16;

/** Room left around the service zone when the map is returned to the whole area. */
const ZONE_PADDING = 32;

/** How much of the city the map shows before a zone arrives. */
const OPENING_ZOOM = 12;

/** Whether the basemap under everything else is drawn, and what the map says when it is not. */
type BasemapState = 'loading' | 'ready' | 'unavailable';

/**
 * What the map says when there is no boundary to draw. It never draws one of its own: a made-up
 * boundary would be a claim about where a rental may end.
 */
const ZONE_ABSENCE: AbsenceCopy = {
  loading: 'Загружаем границу зоны…',
  none: 'Зона обслуживания не задана',
  unreachable: 'Граница зоны не загружена',
};

/** What the map says when the archive beneath it could not be read, since it cannot draw without it. */
const BASEMAP_ABSENCE = 'Подложка карты недоступна: архив карты не загружен';

/** The origin the page was served from, which the map's own addresses are relative to. */
const ORIGIN = window.location.origin;

/**
 * The whole style, built once rather than on every render: it is the same document each time, and
 * building it again would hand the map a second one to compare against the first.
 */
const BASEMAP = basemapStyle(ORIGIN);

type FleetMapProps = {
  vehicles: readonly Vehicle[];
  zones: Found<readonly Zone[]>;
  onRetryZones: () => void;
  selectedId: string | undefined;
  onSelect: (vehicleId: string) => void;
};

/**
 * FleetMap is the main screen. It draws the map of Bishkek the installation serves, the service zone
 * and one marker per vehicle of the current result, and it makes no request of its own beyond the
 * map: everything it shows is passed to it, so a resource that failed to load is explained rather
 * than invented.
 *
 * The basemap is the one thing that can be missing while everything else works, which is why the
 * container says which of the two it is showing: an empty pane would explain nothing.
 */
export function FleetMap({ vehicles, zones, onRetryZones, selectedId, onSelect }: FleetMapProps) {
  const container = useRef<HTMLDivElement>(null);
  const map = useRef<MapLibreMap>(null);
  const markers = useRef<Map<string, VehicleMarker>>(new Map());
  const [basemap, setBasemap] = useState<BasemapState>('loading');
  // The map instance whose style has arrived, which is the one a source or a layer may be added to. It
  // is kept apart from the reading above because that reading outlives the map it was made about: an
  // instance that has just been created would otherwise be judged by the one before it and told it may
  // draw into a style of its own that has not arrived.
  const [styling, setStyling] = useState<MapLibreMap | null>(null);
  const [zoneBounds, setZoneBounds] = useState<LngLatBounds>();
  const drawn = zones.state === 'found' ? zones.value : EMPTY_ZONES;

  useEffect(() => {
    if (container.current === null) return undefined;
    const created = new MapLibreMap({
      container: container.current,
      style: BASEMAP,
      center: BASEMAP_CENTRE,
      zoom: OPENING_ZOOM,
    });
    map.current = created;

    // The style is what carries the basemap, and it has loaded once the archive has answered, so this
    // event rather than a timer is what says the map is drawn. Whether it has already happened is
    // asked as well, because a map can be drawn before the listener that would have heard about it.
    const styleDrawn = () => setBasemap('ready');
    if (created.isStyleLoaded()) styleDrawn();
    created.on('load', styleDrawn);
    // A request for the archive that fails is the one thing that leaves the pane empty, and it is
    // reported by the protocol that made it rather than recognised in the map's own error messages.
    const stopListening = onArchiveFailure(() => setBasemap('unavailable'));

    return () => {
      stopListening();
      markers.current.clear();
      created.remove();
      map.current = null;
    };
  }, []);

  // A pane laid out or resized after the map was created has to say so, or every later fit is computed
  // against the rectangle the map remembers rather than the one it has.
  useEffect(() => {
    if (container.current === null) return undefined;
    const resized = new ResizeObserver(() => map.current?.resize());
    resized.observe(container.current);
    return () => resized.disconnect();
  }, []);

  const showWholeZone = useCallback(() => {
    if (map.current === null || zoneBounds === undefined) return;
    map.current.resize();
    map.current.fitBounds(zoneBounds, { padding: ZONE_PADDING });
  }, [zoneBounds]);

  useEffect(() => {
    const shown = map.current;
    if (shown === null) return undefined;
    const arrived = () => setStyling(shown);
    if (shown.isStyleLoaded()) arrived();
    shown.on('load', arrived);
    return () => {
      setStyling((current) => (current === shown ? null : current));
    };
  }, []);

  useEffect(() => {
    if (styling === null) return;
    putServiceZones(styling, drawn);
    setZoneBounds(serviceZoneBounds(drawn));
  }, [drawn, styling]);

  // The first zone to arrive decides the opening view, which is the whole service area.
  const framed = useRef(false);
  useEffect(() => {
    if (framed.current || zoneBounds === undefined) return;
    framed.current = true;
    showWholeZone();
  }, [zoneBounds, showWholeZone]);

  useEffect(() => {
    if (map.current === null || basemap === 'loading') return;
    syncVehicleMarkers(map.current, markers.current, { vehicles, selectedId }, onSelect);
  }, [vehicles, selectedId, onSelect, basemap]);

  useSelectedVehicleView(map, vehicles, selectedId);

  return (
    <div className="map">
      <div className="map-canvas" ref={container} role="application" aria-label="Карта парка" data-basemap={basemap} />
      <div className="map-caption">{BASEMAP_CAPTION}</div>
      <div className="map-controls">
        <button className="map-action" type="button" onClick={showWholeZone} disabled={zoneBounds === undefined}>
          Показать зону
        </button>
        <ResourceNotice found={zones} copy={ZONE_ABSENCE} onRetry={onRetryZones} />
      </div>
      {basemap === 'unavailable' && <p className="map-basemap-notice">{BASEMAP_ABSENCE}</p>}
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
  map: RefObject<MapLibreMap | null>,
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
    map.current.flyTo({ center: selected.position.coordinates, zoom: SELECTED_VEHICLE_ZOOM });
  }, [map, vehicles, selectedId]);
}
