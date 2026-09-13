import { useCallback, useEffect, useState } from 'react';
import { afterFailure, afterSuccess, type Resource } from './Resource';

/** Loads one resource, abandoning the attempt when the signal is aborted. */
export type LoadResource<T> = (signal: AbortSignal) => Promise<T>;

/** One resource together with the way a person asks for it again. */
export type ResourceHandle<T> = { resource: Resource<T>; retry: () => void };

/**
 * useResource keeps one resource up to date on its own. A refresh interval makes it reload without
 * being asked, which is how a vehicle that has just been released or has just gone stale reaches
 * the screen; a resource with no interval is loaded once and then only when a person retries.
 *
 * Retrying does not clear what is already on screen: the marked-stale snapshot stays until an
 * attempt actually succeeds.
 */
export function useResource<T>(load: LoadResource<T>, refreshMilliseconds?: number): ResourceHandle<T> {
  const [resource, setResource] = useState<Resource<T>>({ phase: 'loading' });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    let abandoned = false;

    async function reload() {
      try {
        const value = await load(controller.signal);
        if (!abandoned) setResource(afterSuccess(value));
      } catch {
        if (!abandoned) setResource(afterFailure);
      }
    }

    void reload();
    const repeat = refreshMilliseconds ? window.setInterval(() => void reload(), refreshMilliseconds) : undefined;

    return () => {
      abandoned = true;
      controller.abort();
      if (repeat !== undefined) window.clearInterval(repeat);
    };
  }, [load, refreshMilliseconds, attempt]);

  const retry = useCallback(() => setAttempt((count) => count + 1), []);

  return { resource, retry };
}
