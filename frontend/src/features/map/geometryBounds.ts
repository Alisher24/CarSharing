import { LngLatBounds } from 'maplibre-gl';
import type { Geometry, Position } from '../../shared/api/catalog';

/**
 * The box a set of published geometries spans. A boundary arrives as a polygon or as several of them,
 * so the walk over it lives here rather than at each place that needs the whole of one.
 */
export function geometryBounds(geometries: readonly Geometry[]): LngLatBounds | undefined {
  // A box that no coordinate has reached yet, so the first one to arrive decides where it opens.
  const bounds = new LngLatBounds();
  let reached = false;
  for (const position of positionsOf(geometries)) {
    bounds.extend(position);
    reached = true;
  }
  return reached ? bounds : undefined;
}

/** Every coordinate a set of geometries is written with, at whatever depth it is nested. */
function positionsOf(geometries: readonly Geometry[]): Position[] {
  return geometries.flatMap((geometry) => coordinatesOf(geometry.coordinates));
}

function coordinatesOf(value: unknown): Position[] {
  if (isPosition(value)) return [value];
  if (!Array.isArray(value)) return [];
  return (value as unknown[]).flatMap(coordinatesOf);
}

/** A coordinate pair, which is where the nesting of a GeoJSON document ends. */
function isPosition(value: unknown): value is Position {
  return Array.isArray(value) && value.length === 2 && value.every((part) => typeof part === 'number');
}
