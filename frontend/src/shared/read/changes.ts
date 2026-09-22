import type { ExactInteger } from '../api/catalog.ts';
import type { DocumentKind } from './readCycle.ts';
import { highestVersion } from './version.ts';

// The separator cannot appear in a resource name or in an identifier, so two different pairs can
// never be read as one key.
const SEPARATOR = '\u0000';

/** One resource whose published state changed. A signal carries no snapshot, only which one moved. */
export type Signal = { resource: DocumentKind; id: string; version: ExactInteger };

/**
 * The highest version seen per resource. A signal is rememberable, not loadable: holding a version
 * here says that the resource moved, never that its new state is known.
 */
export type ObservedVersions = ReadonlyMap<string, string>;

/** What one signal adds to the versions already seen. */
export function recordSignal(observed: ObservedVersions, signal: Signal): ObservedVersions {
  const next = new Map(observed);
  const key = signalKey(signal);

  next.set(key, highestVersion(observed.get(key) ?? signal.version, signal.version));
  return next;
}

/**
 * coalesceSignals folds one window of signals into the highest version each resource reached. A
 * batch of changes to one resource is therefore one read of it rather than one per change.
 */
export function coalesceSignals(signals: readonly Signal[]): ObservedVersions {
  return signals.reduce<ObservedVersions>(recordSignal, new Map());
}

/** The resources a window of signals is about, each named once however often it changed. */
export function changedResources(signals: readonly Signal[]): readonly DocumentKind[] {
  return [...new Set(signals.map((signal) => signal.resource))];
}

/** One resource and one identifier, which is how the stream addresses a change. */
export function signalKey(signal: Signal): string {
  return `${signal.resource}${SEPARATOR}${signal.id}`;
}
