import { randomUUID } from 'node:crypto';
import type { ServerResponse } from 'node:http';
import react from '@vitejs/plugin-react';
import { defineConfig, type Plugin } from 'vite';

const DEV_SERVER_PORT = 5173;
const DEV_API_TARGET = 'http://api:8080';
const RESOURCE_NOT_FOUND = 'RESOURCE_NOT_FOUND';
const RESOURCE_NOT_FOUND_MESSAGE = 'Resource not found';
const HTTP_NOT_FOUND = 404;

// The backend serves these paths on its own port. Answering them here as well would send a browser
// to a route the development server cannot serve, so the dev server retires them explicitly.
const RETIRED_ROUTE_PATTERN = /^\/(health|internal)(\/|\?|$)/;

// Answers a retired route itself and reports whether it handled the request.
function refuseRetiredRoute(requestUrl: string | undefined, response: ServerResponse): boolean {
  if (!RETIRED_ROUTE_PATTERN.test(requestUrl ?? '')) return false;

  const requestId = randomUUID();
  response.writeHead(HTTP_NOT_FOUND, {
    'Content-Type': 'application/json',
    'Cache-Control': 'no-store',
    'X-Request-ID': requestId,
  });
  response.end(
    JSON.stringify({
      code: RESOURCE_NOT_FOUND,
      message: RESOURCE_NOT_FOUND_MESSAGE,
      request_id: requestId,
    }),
  );

  return true;
}

function retiredRoutesPlugin(): Plugin {
  return {
    name: 'retired-routes',
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if (refuseRetiredRoute(request.url, response)) return;
        next();
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), retiredRoutesPlugin()],
  server: {
    port: DEV_SERVER_PORT,
    strictPort: true,
    proxy: { '/api': { target: DEV_API_TARGET } },
  },
});
