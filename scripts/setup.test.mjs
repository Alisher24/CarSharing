import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, writeFile, unlink, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setup } from './setup.mjs';

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
    assert.equal(await readFile(password, 'utf8'), original);
    assert.equal(await readFile(join(root, '.env'), 'utf8'), 'APP_PORT=8181\n');
    await unlink(password);
    await assert.rejects(setup(root), /missing secret files/);
  } finally { await rm(root, { recursive: true, force: true }); }
});
