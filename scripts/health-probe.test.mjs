// A container's health check, the process it probes and the caller that waits for the same answer
// cannot import one another's values, so this check keeps the three saying the same thing: the path a
// caller waits on is the path the API serves, the mark the worker leaves is written where the service
// mounts a writable /tmp, and every probe a service names is one the command answers.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';

const COMPOSE = readFileSync(new URL('../compose.yaml', import.meta.url), 'utf8');
const SERVICE = readFileSync(new URL('./service.mjs', import.meta.url), 'utf8');
const HEALTHCHECK = readFileSync(new URL('../backend/cmd/healthcheck/main.go', import.meta.url), 'utf8');
const READINESS_ROUTE = readFileSync(
  new URL('../backend/internal/platform/httpapi/readiness.go', import.meta.url),
  'utf8',
);
const HEARTBEAT = readFileSync(new URL('../backend/internal/platform/heartbeat/heartbeat.go', import.meta.url), 'utf8');
const READINESS_PROBE = readFileSync(new URL('../backend/cmd/healthcheck/ready.go', import.meta.url), 'utf8');
const HEARTBEAT_PROBE = readFileSync(new URL('../backend/cmd/healthcheck/heartbeat.go', import.meta.url), 'utf8');

/** One value a declaration states, as the string it is written with. */
function stated(declaration, pattern, what) {
  const found = declaration.match(pattern);
  assert.ok(found, `no ${what} this check can read`);
  return found[1];
}

/** The whole `service:` block of compose.yaml, which is what one service declares. */
function serviceBlock(service) {
  const block = COMPOSE.match(new RegExp(`\\n {2}${service}:\\n([\\s\\S]*?)(?=\\n {2}\\w|\\n\\w|$)`));
  assert.ok(block, `compose.yaml declares no ${service} service`);
  return block[1];
}

describe('a container probe reads what its process declares', () => {
  test('waits on the readiness path the API serves', () => {
    const waited = stated(SERVICE, /READINESS_PATH = '([^']+)'/, 'READINESS_PATH in service.mjs');
    const served = stated(READINESS_ROUTE, /ReadyPath = "([^"]+)"/, 'ReadyPath in httpapi/readiness.go');
    assert.equal(waited, served, 'a caller waits on a path the API does not serve');
    assert.match(READINESS_PROBE, /httpapi\.ReadyPath/, 'the readiness probe states its own path');
  });

  test('reads the mark where the service writes it', () => {
    const mark = stated(HEARTBEAT, /Path = "([^"]+)"/, 'Path in platform/heartbeat');
    assert.match(HEARTBEAT_PROBE, /heartbeat\.Fresh\(heartbeat\.Path/, 'the heartbeat probe reads another path');

    // The process is not root, so a mount nobody stated the mode of is one it cannot write to: the
    // declaration has to say that the directory is world-writable, which is what /tmp is for.
    const directory = mark.slice(0, mark.lastIndexOf('/'));
    assert.match(
      serviceBlock('worker'),
      new RegExp(`- ${directory}:mode=1777`),
      `the worker does not declare ${directory} as a writable mount, so the mark it writes has nowhere to go`,
    );
  });

  test('names the probe each service answers, and no probe the command does not', () => {
    const answered = [...HEALTHCHECK.matchAll(/^\t"(\w+)":\s+\w+Probe,$/gm)].map(([, name]) => name);
    assert.ok(answered.length > 0, 'the healthcheck command declares no probe');

    const named = [];
    for (const [service, probe] of [
      ['api', 'ready'],
      ['mailstub', 'ready'],
      ['worker', 'heartbeat'],
    ]) {
      assert.match(
        serviceBlock(service),
        new RegExp(`test: \\[CMD, /usr/local/bin/healthcheck, ${probe}\\]`),
        `${service} does not run the ${probe} probe`,
      );
      named.push(probe);
    }

    for (const probe of named) {
      assert.ok(answered.includes(probe), `compose.yaml names the ${probe} probe, which the command does not answer`);
    }
  });
});
