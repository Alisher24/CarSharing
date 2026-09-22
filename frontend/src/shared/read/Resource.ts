/**
 * What the interface knows about one independently loaded resource. Each resource carries its own
 * value: the fleet failing must not hide a service zone that did load, and a zone failing must not
 * empty the map of vehicles.
 *
 * The stale phase is what keeps the last successful snapshot on screen after the service stops
 * answering. It is deliberately distinct from a first failure, where there is nothing to show and
 * the person is offered another attempt instead of an empty result.
 */
export type Resource<T> =
  | { phase: 'loading' }
  | { phase: 'ready'; value: T; loadedAt: Date }
  | { phase: 'stale'; value: T; loadedAt: Date }
  | { phase: 'failed' };

/** The last value a resource successfully loaded, or undefined when it never loaded one. */
export function loadedValue<T>(resource: Resource<T>): T | undefined {
  return resource.phase === 'ready' || resource.phase === 'stale' ? resource.value : undefined;
}

/** A successful load always replaces what came before it and clears any staleness. */
export function afterSuccess<T>(value: T): Resource<T> {
  return { phase: 'ready', value, loadedAt: new Date() };
}

/**
 * A failed load keeps whatever was last loaded and marks it stale, together with the moment it was
 * loaded at. With nothing loaded yet there is nothing to keep, and the failure is the whole state.
 */
export function afterFailure<T>(previous: Resource<T>): Resource<T> {
  const value = loadedValue(previous);
  if (value === undefined || previous.phase === 'loading' || previous.phase === 'failed') {
    return { phase: 'failed' };
  }
  return { phase: 'stale', value, loadedAt: previous.loadedAt };
}
