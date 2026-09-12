import { useEffect, useState } from 'react';
import { AccountPanel } from '../features/account/AccountPanel';
import { getHealth, type ReadyStatus } from '../shared/api/health';

type Connection = { state: 'loading' } | { state: 'error' } | { state: 'ready'; data: ReadyStatus };

const HEALTH_REQUEST_TIMEOUT_MS = 8000;

const CONNECTION_TEXT: Record<Connection['state'], { title: string; description: string; action: string }> = {
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

export function App() {
  const [connection, setConnection] = useState<Connection>({ state: 'loading' });
  const [retryCount, setRetryCount] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), HEALTH_REQUEST_TIMEOUT_MS);
    let active = true;
    setConnection({ state: 'loading' });
    getHealth(controller.signal)
      .then((data) => { if (active) setConnection({ state: 'ready', data }); })
      .catch(() => { if (active) setConnection({ state: 'error' }); })
      .finally(() => window.clearTimeout(timeout));
    return () => { active = false; controller.abort(); window.clearTimeout(timeout); };
  }, [retryCount]);

  const data = connection.state === 'ready' ? connection.data : null;
  const checkedAt = data && new Intl.DateTimeFormat('ru-RU', {
    hour: '2-digit', minute: '2-digit', second: '2-digit', timeZone: data.timezone,
  }).format(new Date(data.server_time));

  const text = CONNECTION_TEXT[connection.state];
  const currency = data?.currency === 'KGS' ? 'Кыргызский сом · KGS' : (data?.currency ?? '—');
  const lastChecked = checkedAt ? `${checkedAt} · Бишкек` : '—';

  return (
    <div className="page">
      <header>
        <a className="brand" href="/" aria-label="CarSharing — главная">
          <span className="brand-mark" aria-hidden="true">c↗</span> CarSharing
        </a>
        <span className="location"><span aria-hidden="true">◉</span> Бишкек</span>
      </header>

      <main>
        <section className="intro" aria-labelledby="title">
          <p className="eyebrow">СВОЙ РИТМ. СВОЙ МАРШРУТ.</p>
          <h1 id="title">Город ближе,<br />чем кажется.</h1>
          <p className="lead">
            Каршеринг для повседневных поездок по Бишкеку.
            <br className="desktop-break" /> Готовимся к первой поездке вместе с вами.
          </p>
          <div className="notice">
            <span className="notice-dot" />Локальная версия · в разработке
          </div>
        </section>

        <AccountPanel />

        <section className="connection" aria-labelledby="connection-title">
          <div className="connection-top">
            <span className="section-label">СВЯЗЬ С СЕРВИСОМ</span>
            <span className="connection-icon" aria-hidden="true">↗</span>
          </div>
          <div role="status" aria-live="polite" aria-atomic="true">
            <h2 id="connection-title">{text.title}</h2>
            <p className="connection-description">{text.description}</p>
          </div>
          <dl>
            <div><dt>Город</dt><dd>{data?.city ?? '—'}</dd></div>
            <div><dt>Валюта</dt><dd>{currency}</dd></div>
            <div><dt>Последняя проверка</dt><dd>{lastChecked}</dd></div>
          </dl>
          <button
            disabled={connection.state === 'loading'}
            onClick={() => setRetryCount((count) => count + 1)}
          >
            {text.action}<span aria-hidden="true">↻</span>
          </button>
        </section>
      </main>

      <footer>
        <span>CarSharing · Бишкек</span>
        <span>Электро · Бензин · Дизель · Гибрид · Газ</span>
      </footer>
    </div>
  );
}
