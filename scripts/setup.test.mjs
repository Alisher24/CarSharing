import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile, unlink, rm } from 'node:fs/promises';
import { randomBytes } from 'node:crypto';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setup } from './setup.mjs';

const credentials = ['simulator_token', 'demo_control_token', 'mailstub_delivery_token', 'mailstub_demo_token', 'cursor_hmac_key', 'mailstub_cursor_hmac_key'];

test('setup migrates a complete legacy installation and refuses a partial migration', async () => {
  const root = await mkdtemp(join(tmpdir(), 'carsharing-setup-'));
  try {
    await mkdir(join(root, '.secrets'));
    await writeFile(join(root, '.env.example'), 'APP_PORT=8080\n');
    await writeFile(join(root, '.env'), 'APP_PORT=8181\n');
    const legacy = randomBytes(32).toString('hex') + '\n';
    for (const name of ['db_admin_password', 'db_app_password', 'internal_token']) {
      await writeFile(join(root, '.secrets', name), name === 'internal_token' ? legacy : randomBytes(32).toString('hex') + '\n');
    }
    await setup(root);
    assert.ok(await readFile(join(root, '.secrets', 'simulator_token'), 'utf8') === legacy, 'legacy token changed');
    assert.equal(await readFile(join(root, '.env'), 'utf8'), 'APP_PORT=8181\n');
    const values = await Promise.all(credentials.map(name => readFile(join(root, '.secrets', name), 'utf8')));
    assert.equal(new Set(values).size, 6);
    await unlink(join(root, '.secrets', 'simulator_token'));
    await assert.rejects(setup(root), /missing secret files/);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test('setup creates independent capability credentials and preserves all of them on rerun', async () => {
  const root = await mkdtemp(join(tmpdir(), 'carsharing-setup-'));
  try {
    await writeFile(join(root, '.env.example'), 'APP_PORT=8080\n');
    await setup(root);
    const values = await Promise.all(credentials.map(name => readFile(join(root, '.secrets', name), 'utf8')));
    assert.equal(new Set(values).size, 6);
    await setup(root);
    const repeated = await Promise.all(credentials.map(name => readFile(join(root, '.secrets', name), 'utf8')));
    // Compare booleans so a failure cannot print credentials into a test artifact.
    assert.ok(values.every((value, index) => value === repeated[index]), 'credentials changed');
    await unlink(join(root, '.secrets', 'cursor_hmac_key'));
    await assert.rejects(setup(root), /missing secret files/);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test('setup preserves credentials and refuses to replace missing secrets on an existing installation', async () => {
  const root = await mkdtemp(join(tmpdir(), 'carsharing-setup-'));
  try {
    await writeFile(join(root, '.env.example'), 'APP_PORT=8080\n');
    await setup(root);
    const password = join(root, '.secrets', 'db_app_password');
    const original = await readFile(password, 'utf8');
    assert.match(original, /^[a-f0-9]{64}\n$/);
    await writeFile(join(root, '.env'), 'APP_PORT=8181\n');
    await setup(root);
    assert.ok(await readFile(password, 'utf8') === original, 'database password changed');
    assert.equal(await readFile(join(root, '.env'), 'utf8'), 'APP_PORT=8181\n');
    await unlink(password);
    await assert.rejects(setup(root), /missing secret files/);
  } finally { await rm(root, { recursive: true, force: true }); }
});
