import { AccountPanel } from '../features/account/AccountPanel';
import { ConnectionCard } from '../features/connection/ConnectionCard';
import { useConnection } from '../features/connection/useConnection';

export function App() {
  const { connection, retry } = useConnection();

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
        <span className="notice-dot" aria-hidden="true" />
        Локальная версия · в разработке
      </div>
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
