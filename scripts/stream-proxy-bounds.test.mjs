// The event source and proxy cannot import one another's values, so this check keeps the streaming
// heartbeat inside the proxy's idle-read budget.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';

const EVENT_STREAM = readFileSync(new URL('../backend/internal/events/streamtiming.go', import.meta.url), 'utf8');
const NGINX = readFileSync(new URL('../infra/nginx.conf', import.meta.url), 'utf8');

function statedSeconds(declaration, pattern, what) {
  const stated = declaration.match(pattern);
  assert.ok(stated, `no ${what} this check can read`);
  return Number(stated[1]);
}

describe('a streaming connection stays visible to the proxy', () => {
  test('sends a quiet frame before the proxy gives up on reading', () => {
    const keepalive = statedSeconds(
      EVENT_STREAM,
      /keepaliveInterval\s*=\s*(\d+)\s*\*\s*time\.Second/,
      'keepaliveInterval in events/streamtiming.go',
    );
    const proxyRead = statedSeconds(NGINX, /proxy_read_timeout\s+(\d+)s?;/, 'proxy_read_timeout in nginx.conf');

    assert.ok(keepalive < proxyRead, `keepalive ${keepalive}s does not precede proxy timeout ${proxyRead}s`);
  });
});
