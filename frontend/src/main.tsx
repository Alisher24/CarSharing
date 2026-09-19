import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import { App } from './app/App';
import { installBasemapProtocol } from './features/map/basemapProtocol';
import { installBasemapWorker } from './features/map/basemapWorker';
import './app/styles.css';
import './app/fleet.css';
import './app/cabinet.css';
import './app/account.css';

// The map is drawn from an archive rather than from tiles, so MapLibre has to be taught the scheme the
// style names, and told where its own worker is, before any map is created: both are read once, and a
// map made first would keep looking in the wrong place.
installBasemapWorker();
installBasemapProtocol();

const container = document.getElementById('root');
if (!container) throw new Error('index.html must provide the #root element');

createRoot(container).render(
  <StrictMode>
    {/* The addresses of the cabinet are real addresses: the proxy answers every path with the
        application, and the browser's own history is what a person moves through them with. */}
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
