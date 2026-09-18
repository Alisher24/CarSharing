import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import { App } from './app/App';
import './app/styles.css';
import './app/fleet.css';
import './app/cabinet.css';
import './app/account.css';

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
