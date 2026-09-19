// The port the application is reached on, declared in three files that cannot import one another: the
// mapping that publishes the frontend states it, the origins the api service allows a mutating request
// from name it, the installation's own file states the value an operator edits, and the backend derives
// the port its listener defaults to and its documented origins from one constant. A browser sends the
// port it reached the application on as the origin of every mutation, so a list counted from another
// port refuses the application itself — which is what this holds together.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';

const COMPOSE = readFileSync(new URL('../compose.yaml', import.meta.url), 'utf8');
const ENVIRONMENT_EXAMPLE = readFileSync(new URL('../.env.example', import.meta.url), 'utf8');
const CONFIGURATION = readFileSync(new URL('../backend/internal/platform/config/config.go', import.meta.url), 'utf8');

/** The port the frontend is published on, which every other declaration of it must agree with. */
const PUBLISHED_PORT = /ports:\s*\["127\.0\.0\.1:\$\{APP_PORT:-(\d+)\}:\d+"\]/;

/** The origins the api service allows, each naming the port the frontend is published on. */
const ALLOWED_ORIGINS =
  /ALLOWED_ORIGINS:\s*http:\/\/127\.0\.0\.1:\$\{APP_PORT:-(\d+)\},http:\/\/localhost:\$\{APP_PORT:-(\d+)\}/;

/** The port the installation's own file states, which an operator edits to move it. */
const DOCUMENTED_PORT = /^APP_PORT=(\d+)$/m;

/** The port the listener defaults to, as the backend declares it. */
const LISTENER_PORT = /DefaultHTTPPort\s*=\s*"(\d+)"/;

/**
 * The ports one declaration states, or a failure naming the declaration this check could not read.
 * Every declaration here states the same port once, except the origins, which state it under both
 * spellings of the loopback address.
 */
function statedPorts(declaration, pattern, what) {
  const stated = declaration.match(pattern);
  assert.ok(stated, `no ${what} this check can read`);
  return stated.slice(1);
}

function publishedPort() {
  return statedPorts(COMPOSE, PUBLISHED_PORT, 'published port of the frontend in compose.yaml')[0];
}

describe('the port the application is reached on', () => {
  test('is the one a mutation may come from, under both spellings of the loopback address', () => {
    const published = publishedPort();
    const [localAddress, localName] = statedPorts(
      COMPOSE,
      ALLOWED_ORIGINS,
      'ALLOWED_ORIGINS of the api service in compose.yaml',
    );

    assert.equal(
      localAddress,
      published,
      `compose.yaml allows http://127.0.0.1:${localAddress} while publishing ${published}`,
    );
    assert.equal(
      localName,
      published,
      `compose.yaml allows http://localhost:${localName} while publishing ${published}`,
    );
  });

  test('is the one the installation publishes', () => {
    const published = publishedPort();
    const [documented] = statedPorts(ENVIRONMENT_EXAMPLE, DOCUMENTED_PORT, 'APP_PORT in .env.example');

    assert.equal(
      documented,
      published,
      `.env.example states APP_PORT=${documented} while compose.yaml publishes ${published}`,
    );
  });

  test('is the one the listener defaults to, and the documented origins are counted from it', () => {
    const published = publishedPort();
    const [listener] = statedPorts(CONFIGURATION, LISTENER_PORT, 'DefaultHTTPPort in the backend configuration');

    assert.equal(listener, published, `the listener defaults to ${listener} while compose.yaml publishes ${published}`);
  });
});
