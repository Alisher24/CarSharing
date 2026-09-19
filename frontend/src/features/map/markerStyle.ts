import type { VehicleStatus } from '../fleet/filters';

/**
 * The custom property a public state is drawn in. The colours themselves live in the stylesheet,
 * beside every other colour of the interface, so the legend, the list and the markers are painted
 * from one declaration: the components set this property, and the stylesheet decides what it is
 * worth.
 */
export function statusColourProperty(status: VehicleStatus): string {
  return `--status-${status.replaceAll('_', '-')}`;
}
