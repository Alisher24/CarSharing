import type { Connection } from './Connection';
import { CONNECTION_COPY } from './connectionCopy';
import { EMPTY_VALUE, currencyLabel, formatCheckedAt } from './serviceStatus';

/**
 * ConnectionCard reports what the last check found: which city and currency the service states for
 * itself, and when it answered. A server time is rendered in the zone the server reported, so the
 * card never states a city and a clock that disagree.
 */
export function ConnectionCard({ connection, onRetry }: { connection: Connection; onRetry: () => void }) {
  const copy = CONNECTION_COPY[connection.state];
  const status = connection.state === 'ready' ? connection.status : null;
  const lastChecked = status ? formatCheckedAt(status.server_time, status.timezone, status.city) : EMPTY_VALUE;

  return (
    <section className="connection" aria-labelledby="connection-title">
      <div className="connection-top">
        <span className="section-label">СВЯЗЬ С СЕРВИСОМ</span>
        <span className="connection-icon" aria-hidden="true">
          ↗
        </span>
      </div>
      <div role="status" aria-live="polite" aria-atomic="true">
        <h2 className="connection-title" id="connection-title">
          {copy.title}
        </h2>
        <p className="connection-description">{copy.description}</p>
      </div>
      <dl className="details">
        <div className="details-row">
          <dt className="details-term">Город</dt>
          <dd className="details-value">{status?.city ?? EMPTY_VALUE}</dd>
        </div>
        <div className="details-row">
          <dt className="details-term">Валюта</dt>
          <dd className="details-value">{status ? currencyLabel(status.currency) : EMPTY_VALUE}</dd>
        </div>
        <div className="details-row">
          <dt className="details-term">Последняя проверка</dt>
          <dd className="details-value">{lastChecked}</dd>
        </div>
      </dl>
      <button className="action-button" disabled={connection.state === 'loading'} onClick={onRetry}>
        {copy.action}
        <span className="action-button-mark" aria-hidden="true">
          ↻
        </span>
      </button>
    </section>
  );
}
