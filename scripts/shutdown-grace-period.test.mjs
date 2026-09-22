// Programs and Compose cannot import one another's values, so this check keeps the shared shutdown
// budget inside every container grace period.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';

const COMPOSE = readFileSync(new URL('../compose.yaml', import.meta.url), 'utf8');
const LIFECYCLE = readFileSync(new URL('../backend/internal/platform/lifecycle/shutdown.go', import.meta.url), 'utf8');
const API_COMMAND = readFileSync(new URL('../backend/cmd/api/main.go', import.meta.url), 'utf8');
const MAILSTUB_COMMAND = readFileSync(new URL('../backend/cmd/mailstub/main.go', import.meta.url), 'utf8');
const HTTP_SERVER = readFileSync(new URL('../backend/internal/platform/httpserver/serve.go', import.meta.url), 'utf8');

function statedSeconds(declaration, pattern, what) {
  const stated = declaration.match(pattern);
  assert.ok(stated, `no ${what} this check can read`);
  return Number(stated[1]);
}

describe('a process stops inside the container grace period', () => {
  test('uses one shutdown budget shorter than every declared grace period', () => {
    const shutdown = statedSeconds(
      LIFECYCLE,
      /ShutdownTimeout\s*=\s*(\d+)\s*\*\s*time\.Second/,
      'ShutdownTimeout in lifecycle/shutdown.go',
    );
    const gracePeriods = [...COMPOSE.matchAll(/stop_grace_period:\s*(\d+)s/g)].map(([, seconds]) => Number(seconds));
    assert.ok(gracePeriods.length > 0, 'compose.yaml declares no stop_grace_period');
    for (const grace of gracePeriods) {
      assert.ok(shutdown < grace, `shutdown budget ${shutdown}s does not fit in stop_grace_period ${grace}s`);
    }

    for (const [command, source] of [
      ['api', API_COMMAND],
      ['mailstub', MAILSTUB_COMMAND],
    ]) {
      assert.match(source, /httpserver\.Serve\(/, `${command} does not use the shared listener lifecycle`);
      assert.doesNotMatch(source, /shutdownTimeout\s*=/, `${command} declares another shutdown budget`);
    }
    assert.match(HTTP_SERVER, /lifecycle\.ShutdownTimeout/, 'HTTP listeners do not use the shared shutdown budget');
  });
});
