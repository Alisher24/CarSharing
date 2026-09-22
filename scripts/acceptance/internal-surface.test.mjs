// The internal surfaces of the assembled stack: what the outside can reach, which capability reaches
// which operation, and what the declared body limit actually does.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { before, describe, test } from 'node:test';
import { MAILBOX_ORIGIN, repositoryRoot } from '../service.mjs';
import { call, waitForReady } from './client.mjs';
import { ALLOWED_ORIGIN, API_URL, MAILSTUB_URL, bearer, internalRequest } from './internalrequest.mjs';

const STATUS_NOT_FOUND = 404;
const STATUS_UNAUTHORIZED = 401;
const STATUS_REQUEST_ENTITY_TOO_LARGE = 413;
const STATUS_OK = 200;
const STATUS_CREATED = 201;
const NOT_FOUND_CODE = 'RESOURCE_NOT_FOUND';
const INTERNAL_AUTHENTICATION_CODE = 'INTERNAL_AUTHENTICATION_REQUIRED';
const BODY_TOO_LARGE_CODE = 'BODY_TOO_LARGE';

/** The declared request body limit of the public surface, in bytes, read from the contract itself. */
const DECLARED_PUBLIC_BODY_LIMIT = 65536;

/** The limit the proxy declares, which nginx cannot import from the contract. */
const PROXY_BODY_LIMIT = '64k';

const SIMULATOR_TICK_PATH = '/internal/v1/simulation/tick';
const DEMO_CONTROL_PATH = '/internal/v1/demo/actions';
const MAIL_DELIVERY_PATH = '/internal/v1/messages';

/** The key the mail stub accepts one letter under, which a check presents for its own probe. */
const DELIVERY_KEY = 'audit-probe';

/** One well-formed command of each internal capability, which a check aims a wrong token at. */
const TICK_BODY = JSON.stringify({ tick_id: '11111111-1111-4111-8111-111111111111' });
const DEMO_ACTION_BODY = JSON.stringify({
  action_id: '22222222-2222-4222-8222-222222222222',
  action: 'set_energy_remaining',
  vehicle_id: '01994342-6ba7-7000-8000-000000000001',
  source_kind: 'battery',
  remaining: '0',
});
const LETTER_BODY = JSON.stringify({
  delivery_key: 'probe',
  to: 'probe@example.test',
  subject: 'probe',
  body: 'probe',
});

/** The token files the stack mounted, one per capability. */
const TOKEN_FILE = {
  simulator: '.secrets/simulator_token',
  demoControl: '.secrets/demo_control_token',
  delivery: '.secrets/mailstub_delivery_token',
  mailstubDemo: '.secrets/mailstub_demo_token',
};

before(() => waitForReady());

describe('the internal surfaces are not reachable from outside', () => {
  test('every internal and health path answers the unknown-path body through the proxy', async () => {
    const unknown = await call('/api/v1/this-path-does-not-exist');
    assert.equal(unknown.status, STATUS_NOT_FOUND, unknown.text);

    const paths = [
      '/internal',
      '/internal/',
      '/internal/v1/simulation/tick',
      '/internal/v1/demo/actions',
      '/internal/v1/messages',
      '/health',
      '/health/live',
      '/health/ready',
      '/health/v1/health/live',
    ];
    for (const path of paths) {
      const answer = await call(path);
      assert.equal(answer.status, STATUS_NOT_FOUND, `${path} answered ${answer.status}: ${answer.text}`);
      assert.deepEqual(
        { ...answer.json, request_id: 'ignored' },
        { ...unknown.json, request_id: 'ignored' },
        `${path} is told apart from an unknown path`,
      );
    }
    process.stdout.write(`internal surface: ${paths.length} paths answered as an unknown one\n`);
  });

  test('a body over the declared limit is refused through the proxy and on the public listener', async () => {
    const oversized = JSON.stringify({
      email: 'oversized@example.test',
      password: 'x'.repeat(DECLARED_PUBLIC_BODY_LIMIT),
    });

    const throughProxy = await call('/api/v1/auth/register', { method: 'POST', body: JSON.parse(oversized) });
    assert.equal(throughProxy.status, STATUS_REQUEST_ENTITY_TOO_LARGE, throughProxy.text);
    assert.equal(throughProxy.json.code, BODY_TOO_LARGE_CODE, throughProxy.text);

    const onTheListener = internalRequest(API_URL, '/api/v1/auth/register', {
      method: 'POST',
      body: oversized,
      origin: ALLOWED_ORIGIN,
    });
    assert.equal(onTheListener.status, STATUS_REQUEST_ENTITY_TOO_LARGE, onTheListener.body);
    assert.equal(onTheListener.json?.code, BODY_TOO_LARGE_CODE, onTheListener.body);
    process.stdout.write(
      `body limit: declared=${DECLARED_PUBLIC_BODY_LIMIT} through_proxy=${throughProxy.status} ` +
        `on_listener=${onTheListener.status}\n`,
    );
  });

  test('a body within the limit is accepted by the same path', async () => {
    const withinLimit = await call('/api/v1/auth/register', {
      method: 'POST',
      body: { email: `within-limit-${Date.now()}@example.test`, password: 'correcthorsebattery' },
    });
    assert.equal(withinLimit.status, STATUS_CREATED, withinLimit.text);
  });
});

describe('the proxy declares the same limit as the contract', () => {
  test('client_max_body_size covers x-body-limit', () => {
    const proxy = readFileSync(join(repositoryRoot, 'infra/nginx.conf'), 'utf8');
    const declared = proxy.match(/client_max_body_size\s+(\S+);/);
    assert.ok(declared, 'the proxy declares no body limit');
    assert.equal(declared[1], PROXY_BODY_LIMIT, 'the proxy limit changed; the contract documents this value');
    assert.equal(
      parseBodyLimit(PROXY_BODY_LIMIT),
      DECLARED_PUBLIC_BODY_LIMIT,
      'the proxy limit does not cover the limit the contract declares',
    );
  });
});

describe('one capability does not call another', () => {
  test('a token of one internal capability is refused the operations of the others', () => {
    const refusals = [
      [
        'the delivery token at the simulator tick',
        apiCall(SIMULATOR_TICK_PATH, { body: TICK_BODY, capability: 'delivery' }),
      ],
      [
        'the demonstration-control token at the simulator tick',
        apiCall(SIMULATOR_TICK_PATH, { body: TICK_BODY, capability: 'demoControl' }),
      ],
      [
        'the delivery token at the demonstration control',
        apiCall(DEMO_CONTROL_PATH, { body: DEMO_ACTION_BODY, capability: 'delivery' }),
      ],
      [
        'the simulator token at the demonstration control',
        apiCall(DEMO_CONTROL_PATH, { body: DEMO_ACTION_BODY, capability: 'simulator' }),
      ],
      ['no token at the simulator tick', apiCall(SIMULATOR_TICK_PATH, { body: TICK_BODY, capability: NO_CAPABILITY })],
      ['an unknown token at the simulator tick', unknownTokenCall(SIMULATOR_TICK_PATH, TICK_BODY)],
      ['an unknown token at the demonstration control', unknownTokenCall(DEMO_CONTROL_PATH, DEMO_ACTION_BODY)],
      ['no token at the mail delivery', mailCall({ body: LETTER_BODY, capability: NO_CAPABILITY })],
      ['the demonstration token at the mail delivery', mailCall({ body: LETTER_BODY, capability: 'mailstubDemo' })],
    ];

    for (const [what, answer] of refusals) {
      assert.equal(answer.status, STATUS_UNAUTHORIZED, `${what} was accepted: ${answer.body}`);
      assert.equal(answer.json?.code, INTERNAL_AUTHENTICATION_CODE, `${what} answered ${answer.body}`);
      for (const leaked of ['TOKEN_FILE', 'simulator_token', 'demo_control_token', 'mailstub_delivery_token']) {
        assert.ok(!answer.body.includes(leaked), `${what} leaked the name of a setting: ${answer.body}`);
      }
    }
    process.stdout.write(`internal surface: ${refusals.length} capability refusals observed\n`);
  });

  test('the right token still reaches its own operation, so the refusals are about the capability', () => {
    const tick = apiCall(SIMULATOR_TICK_PATH, { body: TICK_BODY, capability: 'simulator' });
    assert.equal(tick.status, STATUS_OK, `the simulator token was refused the tick: ${tick.body}`);
    assert.ok(tick.json?.tick_id, tick.body);

    const delivery = mailCall({ body: LETTER_BODY, capability: 'delivery' });
    assert.ok(
      [STATUS_OK, STATUS_CREATED, 400, 409, 422].includes(delivery.status),
      `the delivery token was refused for a reason other than its capability: ${delivery.body}`,
    );
  });
});

/** One command presented with a credential no capability of this installation holds. */
function unknownTokenCall(path, body) {
  return internalRequest(API_URL, path, {
    method: 'POST',
    body,
    headers: bearer('not-a-real-token'),
  });
}

describe('the mailbox is a read-only surface on the loopback address', () => {
  test('answers the inbox without a session and serves no mutating operation', async () => {
    const inbox = await fetch(`${MAILBOX_ORIGIN}/api/v1/messages`);
    assert.equal(inbox.status, STATUS_OK, await inbox.text());

    for (const path of [MAIL_DELIVERY_PATH, DEMO_CONTROL_PATH]) {
      const answer = await fetch(`${MAILBOX_ORIGIN}${path}`, { method: 'POST', body: '{}' });
      assert.ok([STATUS_NOT_FOUND, 405].includes(answer.status), `the mailbox answered ${answer.status} to ${path}`);
    }
  });
});

/** Reads one mounted capability token, which a check presents as the service that holds it. */
function token(capability) {
  if (capability === NO_CAPABILITY) return '';
  return readFileSync(join(repositoryRoot, TOKEN_FILE[capability]), 'utf8').trim();
}

/** How a check names the case of presenting no credential at all. */
const NO_CAPABILITY = 'none';

/** One command sent to the API's internal listener under one capability. */
function apiCall(path, { body, capability }) {
  return internalRequest(API_URL, path, { method: 'POST', body, headers: bearer(token(capability)) });
}

/** One delivery sent to the mail stub's internal listener under its delivery capability. */
function mailCall({ body, capability }) {
  return internalRequest(MAILSTUB_URL, MAIL_DELIVERY_PATH, {
    method: 'POST',
    body,
    headers: { ...bearer(token(capability)), 'Delivery-Key': DELIVERY_KEY },
  });
}

/** A body limit stated the way nginx states it, in bytes. */
function parseBodyLimit(text) {
  const match = text.match(/^(\d+)([km])?$/);
  assert.ok(match, `the body limit ${text} is not a size`);
  const scale = { k: 1024, m: 1024 * 1024 }[match[2]] ?? 1;
  return Number(match[1]) * scale;
}
