import { INTERFACE_LOCALE } from '../locale.ts';
import { getHealthReady } from './generated/sdk.gen';
import type { ReadyStatus } from './generated/types.gen';

export type { ReadyStatus } from './generated/types.gen';

// The view formats the server time in the interface locale and in the zone the service states, so a
// zone this browser cannot format in is rejected here rather than surfacing later as a failure in the
// middle of a render.
function assertKnownTimeZone(timeZone: string): void {
  try {
    new Intl.DateTimeFormat(INTERFACE_LOCALE, { timeZone });
  } catch {
    throw new Error(`Unknown time zone: ${timeZone}`);
  }
}

export async function getHealth(signal: AbortSignal): Promise<ReadyStatus> {
  const { data: readiness } = await getHealthReady({ signal, cache: 'no-store', throwOnError: true });
  if (readiness.status !== 'ok' || Number.isNaN(Date.parse(readiness.server_time))) {
    throw new Error('Invalid service response');
  }

  assertKnownTimeZone(readiness.timezone);

  return readiness;
}
