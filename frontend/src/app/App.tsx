import { useEffect, useState } from 'react';
import { AccountPanel } from '../features/account/AccountPanel';
import { getHealth, type ReadyStatus } from '../shared/api/health';

type Connection = { state: 'loading' } | { state: 'ready'; status: ReadyStatus } | { state: 'error' };

type ConnectionCopy = { title: string; description: string; action: string };

const HEALTH_REQUEST_TIMEOUT_MS = 8000;
const EMPTY_VALUE = '—';
const BISHKEK_TIME_ZONE = 'Asia/Bishkek';

const CONNECTION_COPY: Record<Connection['state'], ConnectionCopy> = {
  loading: {
    title: 'Проверяем соединение…',
    description: 'Запрашиваем актуальные данные.',
    action: 'Проверить соединение',
  },
  ready: {
    title: 'Сервис на связи',
    description: 'Соединение установлено. Выбор автомобиля и бронирование появятся в следующих версиях.',
    action: 'Проверить соединение',
  },
  error: {
    title: 'Нет связи с сервисом',
    description: 'Не удалось получить ответ. Проверьте, запущен ли сервис, и повторите попытку.',
    action: 'Повторить попытку',
  },
};

const CURRENCY_LABELS: Record<string, string> = {
  KGS: 'Кыргызский сом · KGS',
};

function formatCheckedAt(serverTime: string): string {
  const checkedAt = new Intl.DateTimeFormat('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    timeZone: BISHKEK_TIME_ZONE,
  }).format(new Date(serverTime));

  return `${checkedAt} · Бишкек`;
}

function currencyLabel(currency: string): string {
  return CURRENCY_LABELS[currency] ?? currency;
}

// A failed check and a slow check are one outcome for the interface: the service did not answer.
async function checkConnection(signal: AbortSignal): Promise<Connection> {
  try {
    return { state: 'ready', status: await getHealth(signal) };
  } catch {
    return { state: 'error' };
  }
}

export function App() {
  const [connection, setConnection] = useState<Connection>({ state: 'loading' });
  const [retryCount, setRetryCount] = useState(0);

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
  }, [retryCount]);

  function retry() {
    setRetryCount((count) => count + 1);
  }

  return (
    <div className="page">
      <AppHeader />
      <main className="page-main">
        <Intro />
        <AccountPanel />
        <ConnectionCard connection={connection} onRetry={retry} />
      </main>
      <AppFooter />
    </div>
  );
}

function AppHeader() {
  return (
    <header className="page-header">
      <a className="brand" href="/" aria-label="CarSharing — главная">
        <span className="brand-mark" aria-hidden="true">
          c↗
        </span>
        CarSharing
      </a>
      <span className="location">
        <span className="location-mark" aria-hidden="true">
          ◉
        </span>
        Бишкек
      </span>
    </header>
  );
}

function Intro() {
  return (
    <section className="intro" aria-labelledby="title">
      <p className="eyebrow">СВОЙ РИТМ. СВОЙ МАРШРУТ.</p>
      <h1 className="intro-title" id="title">
        Город ближе,
        <br />
        чем кажется.
      </h1>
      <p className="lead">
        Каршеринг для повседневных поездок по Бишкеку.
        <br className="desktop-break" /> Готовимся к первой поездке вместе с вами.
      </p>
      <div className="notice">
        <span className="notice-dot" />
        Локальная версия · в разработке
      </div>
    </section>
  );
}

function ConnectionCard({ connection, onRetry }: { connection: Connection; onRetry: () => void }) {
  const copy = CONNECTION_COPY[connection.state];
  const status = connection.state === 'ready' ? connection.status : null;
  const lastChecked = status ? formatCheckedAt(status.server_time) : EMPTY_VALUE;

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

function AppFooter() {
  return (
    <footer className="page-footer">
      <span>CarSharing · Бишкек</span>
      <span>Электро · Бензин · Дизель · Гибрид · Газ</span>
    </footer>
  );
}
