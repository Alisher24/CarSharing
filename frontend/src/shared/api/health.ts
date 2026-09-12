import { getHealthReady } from './generated/sdk.gen';
import type { ReadyStatus } from './generated/types.gen';

export type { ReadyStatus } from './generated/types.gen';

// The view formats the server time in this zone, so an unknown zone is rejected here rather than
// surfacing later as a formatting failure in the middle of rendering.
function assertKnownTimeZone(timeZone: string): void {
  try {
    new Intl.DateTimeFormat('ru', { timeZone });
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
