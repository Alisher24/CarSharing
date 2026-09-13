import type { EventsConnection } from '../events/useEventStream';
import type { Connection } from './useConnection';

/** What the strip says while the subscription has not been established yet. */
const CONNECTING = 'Подключаемся к обновлениям';

/** What it says once the subscription has ended and is being retried. */
const DELAYED = 'Обновления задерживаются';

type StreamNoticeProps = { stream: EventsConnection; connection: Connection };

/**
 * StreamNotice says how the change stream is doing, and only when that is news. A stream that is
 * merely still connecting is not a fault, and a service this browser cannot reach at all is already
 * reported as stale data rather than as a delay, so neither is repeated here.
 */
export function StreamNotice({ stream, connection }: StreamNoticeProps) {
  const serviceReachable = connection.phase === 'ready' || connection.phase === 'stale';
  if (stream === 'connected' || !serviceReachable) return null;

  return (
    <span className="stream-notice" data-stream={stream} role="status">
      <span className="stream-notice-dot" aria-hidden="true" />
      {stream === 'connecting' ? CONNECTING : DELAYED}
    </span>
  );
}
