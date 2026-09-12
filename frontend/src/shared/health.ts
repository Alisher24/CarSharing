import { getHealthReady } from './api/generated/sdk.gen';
import type { ReadyStatus } from './api/generated/types.gen';

export type Health = ReadyStatus;

export async function getHealth(signal: AbortSignal): Promise<Health> {
  const { data } = await getHealthReady({ signal, cache: 'no-store', throwOnError: true });
  if (data.status !== 'ok' || Number.isNaN(Date.parse(data.server_time))) {
    throw new Error('Invalid service response');
  }
  new Intl.DateTimeFormat('ru', { timeZone: data.timezone }).format();
  return data;
}
