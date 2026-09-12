import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile, unlink, rm } from 'node:fs/promises';
import { randomBytes } from 'node:crypto';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import {
  setup,
  CAPABILITY_SECRETS,
  ENVIRONMENT_FILE,
  LEGACY_SECRET,
  SECRETS_DIRECTORY,
  SECRET_BYTES,
  SECRET_VALUE_PATTERN,
} from './setup.mjs';

const DATABASE_SECRET_NAMES = ['db_admin_password', 'db_app_password'];
const EXISTING_ENVIRONMENT = 'APP_PORT=8181\n';
const TEMPLATE_ENVIRONMENT = 'APP_PORT=8080\n';
const MISSING_SECRET_PATTERN = /missing secret files/;

function newSecretValue() {
  return randomBytes(SECRET_BYTES).toString('hex') + '\n';
}

function secretPath(root, name) {
  return join(root, SECRETS_DIRECTORY, name);
}

function readGeneratedSecrets(root, names) {
  return Promise.all(names.map((name) => readFile(secretPath(root, name), 'utf8')));
}

/** Creates a scratch installation so a test never reaches the repository's own secrets. */
async function createInstallation() {
  const root = await mkdtemp(join(tmpdir(), 'carsharing-setup-'));
  await writeFile(join(root, '.env.example'), TEMPLATE_ENVIRONMENT);
  return root;
}

async function writeSecrets(root, values) {
  for (const [name, value] of Object.entries(values)) {
    await writeFile(secretPath(root, name), value);
  }
}

test('setup migrates a complete legacy installation and refuses a partial migration', async () => {
  const root = await createInstallation();
  try {
    await mkdir(join(root, SECRETS_DIRECTORY));
    await writeFile(join(root, ENVIRONMENT_FILE), EXISTING_ENVIRONMENT);
    // A legacy installation holds the two database passwords and the single token that covered
    // every capability the service now splits into separate credentials.
    const legacyValue = newSecretValue();
    await writeSecrets(root, {
      db_admin_password: newSecretValue(),
      db_app_password: newSecretValue(),
      [LEGACY_SECRET]: legacyValue,
    });

    await setup(root);
    assert.equal(await readFile(secretPath(root, 'simulator_token'), 'utf8'), legacyValue, 'legacy token changed');
    assert.equal(await readFile(join(root, ENVIRONMENT_FILE), 'utf8'), EXISTING_ENVIRONMENT);

    const capabilityValues = await readGeneratedSecrets(root, CAPABILITY_SECRETS);
    assert.equal(new Set(capabilityValues).size, CAPABILITY_SECRETS.length);

    await unlink(secretPath(root, 'simulator_token'));
    await assert.rejects(setup(root), MISSING_SECRET_PATTERN);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test('setup creates independent capability credentials and preserves all of them on rerun', async () => {
  const root = await createInstallation();
  try {
    await setup(root);
    const firstValues = await readGeneratedSecrets(root, CAPABILITY_SECRETS);
    assert.equal(new Set(firstValues).size, CAPABILITY_SECRETS.length);

    await setup(root);
    const repeatedValues = await readGeneratedSecrets(root, CAPABILITY_SECRETS);
    // Only the comparison result is reported, so a failure cannot print credentials into a test artifact.
    assert.ok(
      firstValues.every((value, index) => value === repeatedValues[index]),
      'credentials changed',
    );

    await unlink(secretPath(root, 'cursor_hmac_key'));
    await assert.rejects(setup(root), MISSING_SECRET_PATTERN);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test('setup preserves credentials and refuses to replace missing secrets on an existing installation', async () => {
  const root = await createInstallation();
  try {
    await setup(root);
    const databasePasswordPath = secretPath(root, 'db_app_password');
    const originalPassword = await readFile(databasePasswordPath, 'utf8');
    assert.match(originalPassword, SECRET_VALUE_PATTERN);

    await writeFile(join(root, ENVIRONMENT_FILE), EXISTING_ENVIRONMENT);
    await setup(root);
    assert.equal(await readFile(databasePasswordPath, 'utf8'), originalPassword, 'database password changed');
    assert.equal(await readFile(join(root, ENVIRONMENT_FILE), 'utf8'), EXISTING_ENVIRONMENT);

    await unlink(databasePasswordPath);
    await assert.rejects(setup(root), MISSING_SECRET_PATTERN);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
