import type { Connection } from './useConnection';

/**
 * How the browser's own link to the service is described. This is the reader's connection, not a
 * vehicle's: a vehicle that has lost its link is reported on its own card and is unaffected by
 * whether this browser can reach the API.
 */
const CONNECTION_LABELS: Record<Connection['phase'], string> = {
  loading: 'Проверяем связь',
  ready: 'Связь с сервисом есть',
  stale: 'Нет связи с сервисом',
  failed: 'Нет связи с сервисом',
};

/** ConnectionIndicator is the compact form the header carries, beside the entrance to an account. */
export function ConnectionIndicator({ connection }: { connection: Connection }) {
  return (
    <span className={`connection-indicator connection-indicator-${connection.phase}`}>
      <span className="connection-indicator-dot" aria-hidden="true" />
      {CONNECTION_LABELS[connection.phase]}
    </span>
  );
}
