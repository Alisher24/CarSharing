import { useEffect, useState } from 'react';
import { getHealth } from '../../shared/api/health';
import type { Connection } from './Connection';

const HEALTH_REQUEST_TIMEOUT_MS = 8000;

// A failed check and a slow check are one outcome for the interface: the service did not answer.
async function checkConnection(signal: AbortSignal): Promise<Connection> {
  try {
    return { state: 'ready', status: await getHealth(signal) };
  } catch {
    return { state: 'error' };
  }
}

/**
 * useConnection asks the service whether it is reachable, and asks again whenever the person
 * retries. The request is abandoned on unmount and on a new attempt, so a slow answer never lands
 * on a screen that has moved on.
 */
export function useConnection() {
  const [connection, setConnection] = useState<Connection>({ state: 'loading' });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    const abortTimer = window.setTimeout(() => controller.abort(), HEALTH_REQUEST_TIMEOUT_MS);
    let mounted = true;

    setConnection({ state: 'loading' });
    checkConnection(controller.signal).then((result) => {
      window.clearTimeout(abortTimer);
      if (mounted) setConnection(result);
    });

    return () => {
      mounted = false;
      window.clearTimeout(abortTimer);
      controller.abort();
    };
  }, [attempt]);

  function retry() {
    setAttempt((count) => count + 1);
  }

  return { connection, retry };
}
