import type { ExactInteger } from '../../shared/api/catalog.ts';
import type { ObservedVersions } from './changes.ts';

/** One published object that states its own version. */
export type Versioned = { id: string; version: ExactInteger };

/** The versions one collection publishes, keyed as the change signals address them. */
export function versionsOf(items: readonly Versioned[]): ObservedVersions {
  return new Map(items.map((item) => [item.id, item.version]));
}
