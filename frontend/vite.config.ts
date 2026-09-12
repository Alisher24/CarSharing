import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { randomUUID } from 'node:crypto';

export default defineConfig({
  plugins: [react(), {
    name: 'private-and-retired-routes',
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if (!/^\/(health|internal)(\/|\?|$)/.test(request.url ?? '')) return next();
        const requestId = randomUUID();
        response.writeHead(404, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', 'X-Request-ID': requestId });
        response.end(JSON.stringify({ code: 'RESOURCE_NOT_FOUND', message: 'Resource not found', request_id: requestId }));
      });
    },
  }],
  server: {
    port: 5173,
    strictPort: true,
    proxy: { '/api': { target: 'http://api:8080' } },
  },
});
