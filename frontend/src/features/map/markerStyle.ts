import type { VehicleStatus } from '../fleet/filters';

/** How large a vehicle is drawn, and how much larger the one a person has selected is. */
export const MARKER_RADIUS = 7;
export const SELECTED_MARKER_RADIUS = 11;

/** How heavy a marker outline is, and how much heavier a selected one is. */
export const MARKER_OUTLINE_WEIGHT = 2;
export const SELECTED_MARKER_OUTLINE_WEIGHT = 3;

/** How solid a marker is filled, so a street underneath stays faintly visible. */
export const MARKER_FILL_OPACITY = 0.9;

/** The colour a selected marker is outlined in, which is the interface's own ink. */
const SELECTED_OUTLINE_PROPERTY = '--color-ink';

/**
 * The custom property a public state is drawn in. The colours themselves live in the stylesheet,
 * beside every other colour of the interface, so the legend, the list and the markers are painted
 * from one declaration.
 */
export function statusColourProperty(status: VehicleStatus): string {
  return `--status-${status.replaceAll('_', '-')}`;
}

/**
 * declaredColour reads a colour back out of the stylesheet. The map library is handed colour
 * strings rather than class names, which is the one place the value has to be resolved in script.
 */
export function declaredColour(property: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(property).trim();
}

export function statusColour(status: VehicleStatus): string {
  return declaredColour(statusColourProperty(status));
}

export function selectedOutlineColour(): string {
  return declaredColour(SELECTED_OUTLINE_PROPERTY);
}
