import type { ExactInteger } from '../api/catalog.ts';
import { NO_VERSION } from './version.ts';
import type { ObservedVersions } from './changes.ts';

/** One published object that states its own version. */
export type Versioned = { id: string; version: ExactInteger };

/** The versions one collection publishes, keyed as the change signals address them. */
export function versionsOf(items: readonly Versioned[]): ObservedVersions {
  return new Map(items.map((item) => [item.id, item.version]));
}

/**
 * The version one collection published for an identifier, or the version that precedes every
 * published one. A collection that does not hold the object has published nothing about it, which is
 * what makes an answer carrying it worth storing.
 */
export function versionIn(items: readonly Versioned[], id: string): string {
  return items.find((item) => item.id === id)?.version ?? NO_VERSION;
}
