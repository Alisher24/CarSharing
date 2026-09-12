import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './app/App';
import './app/styles.css';

const container = document.getElementById('root');
if (!container) throw new Error('index.html must provide the #root element');

createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
