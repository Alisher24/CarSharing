import type { FleetSnapshot } from '../../shared/api/catalog.ts';
import { RETRY_ACTION } from '../../shared/copy.ts';
import type { Resource } from '../../shared/read/Resource.ts';
import { clockMoment } from '../../shared/time.ts';

/** What the strip says while the first reading is still on its way. */
const LOADING = 'Загружаем парк…';

/** What it says once the service has stopped answering but a reading is still on screen. */
const STALE = 'Данные устарели';

/** What it says when nothing has ever been read, which is not the same as an empty fleet. */
const NEVER_LOADED = 'Не удалось загрузить парк';

type FleetStatusProps = { resource: Resource<FleetSnapshot>; onRetry: () => void };

/**
 * FleetStatus says how much the fleet on screen can be trusted. A failure after a successful
 * reading keeps that reading and marks it, together with when it was taken; a failure before any
 * reading offers another attempt instead of an empty parking lot.
 */
export function FleetStatus({ resource, onRetry }: FleetStatusProps) {
  if (resource.phase === 'loading') {
    return <p className="fleet-status">{LOADING}</p>;
  }

  if (resource.phase === 'ready') {
    return <p className="fleet-status">Обновлено в {clockMoment(resource.loadedAt)}</p>;
  }

  return (
    <p className="fleet-status fleet-status-warning" role="status">
      {warningText(resource)}
      <button className="fleet-retry" type="button" onClick={onRetry}>
        {RETRY_ACTION}
      </button>
    </p>
  );
}

/** A snapshot that is still on screen says when it was taken; one that never arrived cannot. */
function warningText(resource: Resource<FleetSnapshot>): string {
  if (resource.phase !== 'stale') return NEVER_LOADED;

  return `${STALE} · последнее обновление в ${clockMoment(resource.loadedAt)}`;
}
