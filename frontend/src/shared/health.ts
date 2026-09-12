export type Health = {
  status: 'ok';
  city: string;
  currency: string;
  timezone: string;
  server_time: string;
};

export async function getHealth(signal: AbortSignal): Promise<Health> {
  const response = await fetch('/api/health', { signal, cache: 'no-store' });
  if (!response.ok) throw new Error('Service unavailable');
  const data: unknown = await response.json();
  if (
    typeof data !== 'object' || data === null ||
    !('status' in data) || data.status !== 'ok' ||
    !('city' in data) || typeof data.city !== 'string' ||
    !('currency' in data) || typeof data.currency !== 'string' ||
    !('timezone' in data) || typeof data.timezone !== 'string' ||
    !('server_time' in data) || typeof data.server_time !== 'string' ||
    Number.isNaN(Date.parse(data.server_time))
  ) throw new Error('Invalid service response');
  // A bad timezone must become a connection error rather than crashing the UI.
  new Intl.DateTimeFormat('ru', { timeZone: data.timezone }).format();
  return {
    status: data.status, city: data.city, currency: data.currency,
    timezone: data.timezone, server_time: data.server_time,
  };
}
