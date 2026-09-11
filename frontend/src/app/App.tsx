import { useEffect, useState } from 'react';
import { getHealth, type Health } from '../shared/health';

type Connection = { state: 'loading' } | { state: 'error' } | { state: 'ready'; data: Health };

export function App() {
  const [connection, setConnection] = useState<Connection>({ state: 'loading' });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 8000);
    let active = true;
    setConnection({ state: 'loading' });
    getHealth(controller.signal)
      .then((data) => { if (active) setConnection({ state: 'ready', data }); })
      .catch(() => { if (active) setConnection({ state: 'error' }); })
      .finally(() => window.clearTimeout(timeout));
    return () => { active = false; controller.abort(); window.clearTimeout(timeout); };
  }, [attempt]);

  const data = connection.state === 'ready' ? connection.data : null;
  const checkedAt = data && new Intl.DateTimeFormat('ru-RU', {
    hour: '2-digit', minute: '2-digit', second: '2-digit', timeZone: data.timezone,
  }).format(new Date(data.server_time));

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
          <p className="lead">Каршеринг для повседневных поездок по Бишкеку.<br className="desktop-break" /> Готовимся к первой поездке вместе с вами.</p>
          <div className="notice"><span className="notice-dot" />Локальная версия · в разработке</div>
        </section>

        <section className="connection" aria-labelledby="connection-title">
          <div className="connection-top"><span className="section-label">СВЯЗЬ С СЕРВИСОМ</span><span className="connection-icon" aria-hidden="true">↗</span></div>
          <div role="status" aria-live="polite" aria-atomic="true">
            <h2 id="connection-title">{connection.state === 'loading' ? 'Проверяем соединение…' : connection.state === 'ready' ? 'Сервис на связи' : 'Нет связи с сервисом'}</h2>
            <p className="connection-description">{connection.state === 'error' ? 'Не удалось получить ответ. Проверьте, запущен ли сервис, и повторите попытку.' : connection.state === 'loading' ? 'Запрашиваем актуальные данные.' : 'Соединение установлено. Выбор автомобиля и бронирование появятся в следующих версиях.'}</p>
          </div>
          <dl>
            <div><dt>Город</dt><dd>{data?.city ?? '—'}</dd></div>
            <div><dt>Валюта</dt><dd>{data?.currency === 'KGS' ? 'Кыргызский сом · KGS' : (data?.currency ?? '—')}</dd></div>
            <div><dt>Последняя проверка</dt><dd>{checkedAt ? `${checkedAt} · Бишкек` : '—'}</dd></div>
          </dl>
          <button disabled={connection.state === 'loading'} onClick={() => setAttempt((n) => n + 1)}>
            {connection.state === 'error' ? 'Повторить попытку' : 'Проверить соединение'}<span aria-hidden="true">↻</span>
          </button>
        </section>
      </main>

      <footer><span>CarSharing · Бишкек</span><span>Электро · Бензин · Дизель · Гибрид · Газ</span></footer>
    </div>
  );
}
