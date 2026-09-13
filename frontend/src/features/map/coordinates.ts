import { LatLng, type LatLngExpression } from 'leaflet';
import type { Position } from '../../shared/api/generated/types.gen';

/**
 * The contract writes a coordinate pair longitude first, as GeoJSON does, and Leaflet takes them
 * latitude first. Every conversion between the two goes through here, so the order is decided once
 * rather than at each call that happens to need a point.
 */
export function toLatLng([longitude, latitude]: Position): LatLng {
  return new LatLng(latitude, longitude);
}

export function toLatLngs(path: readonly Position[]): LatLngExpression[] {
  return path.map(toLatLng);
}
