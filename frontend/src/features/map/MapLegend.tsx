import type { Zone } from '../../shared/api/catalog';
import { FILTERABLE_STATUSES } from '../fleet/filters';
import { STATUS_LABELS } from '../fleet/fleetCopy';
import { statusColourProperty } from './markerStyle';

/** What the legend calls a boundary the service has not published. */
const UNNAMED_ZONE = 'Зона обслуживания';

/**
 * MapLegend explains the markers using the same colours they are drawn from, and names the service
 * area the way the operator published it, so the boundary is called demonstrational on the map
 * rather than only in the caption under it.
 */
export function MapLegend({ zones }: { zones: readonly Zone[] }) {
  return (
    <div className="map-legend">
      <p className="map-legend-title">Обозначения</p>
      <ul className="map-legend-items">
        {FILTERABLE_STATUSES.map((status) => (
          <li className="map-legend-item" key={status}>
            <span
              className="map-legend-dot"
              style={{ background: `var(${statusColourProperty(status)})` }}
              aria-hidden="true"
            />
            {STATUS_LABELS[status]}
          </li>
        ))}
        {zoneNames(zones).map((name) => (
          <li className="map-legend-item" key={name}>
            <span className="map-legend-zone" aria-hidden="true" />
            {name}
          </li>
        ))}
      </ul>
    </div>
  );
}

function zoneNames(zones: readonly Zone[]): string[] {
  if (zones.length === 0) return [UNNAMED_ZONE];
  return zones.map((zone) => zone.name);
}
