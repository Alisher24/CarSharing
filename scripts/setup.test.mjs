import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtemp, mkdir, readFile, writeFile, unlink, rm } from 'node:fs/promises';
import { randomBytes } from 'node:crypto';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
import {
  setup,
  CAPABILITY_SECRETS,
  ENVIRONMENT_FILE,
  LEGACY_SECRET,
  SECRETS_DIRECTORY,
  SECRET_BYTES,
  SECRET_VALUE_PATTERN,
} from './setup.mjs';

const DATABASE_SECRET_NAMES = ['db_admin_password', 'db_app_password', 'mailstub_app_password'];

// The two database passwords a legacy installation holds, which predate the mail stub's role.
const LEGACY_DATABASE_SECRET_NAMES = ['db_admin_password', 'db_app_password'];
const EXISTING_ENVIRONMENT = 'APP_PORT=8181\n';
const TEMPLATE_ENVIRONMENT = 'APP_PORT=8080\n';
const MISSING_SECRET_PATTERN = /missing secret files/;

// The gate setup installs, which is a setting of the working copy rather than of an installation.
const HOOKS_DIRECTORY = '.githooks';
const PRE_COMMIT_HOOK = 'pre-commit';
const NOT_A_WORKING_COPY_PATTERN = /not a git repository/;

const runGit = promisify(execFile);

function newSecretValue() {
  return randomBytes(SECRET_BYTES).toString('hex') + '\n';
}

function secretPath(root, name) {
  return join(root, SECRETS_DIRECTORY, name);
}

function readGeneratedSecrets(root, names) {
  return Promise.all(names.map((name) => readFile(secretPath(root, name), 'utf8')));
}

/** A directory holding what a working copy holds: the environment template and the tracked gate. */
async function createCopy() {
  const root = await mkdtemp(join(tmpdir(), 'carsharing-setup-'));
  await writeFile(join(root, '.env.example'), TEMPLATE_ENVIRONMENT);
  await mkdir(join(root, HOOKS_DIRECTORY));
  await writeFile(join(root, HOOKS_DIRECTORY, PRE_COMMIT_HOOK), '#!/bin/sh\nset -eu\n');
  return root;
}

/** Creates a scratch installation, which is a working copy, so a test never reaches the repository's. */
async function createInstallation() {
  const root = await createCopy();
  await runGit('git', ['init', '--quiet', root]);
  return root;
}

/** The value Git holds for one setting of a working copy. */
async function gitSetting(root, key) {
  const { stdout } = await runGit('git', ['-C', root, 'config', '--local', '--get', key]);
  return stdout.trim();
}

async function writeSecrets(root, values) {
  for (const [name, value] of Object.entries(values)) {
    await writeFile(secretPath(root, name), value);
  }
}

test('setup installs the gate the working copy commits through', async () => {
  const root = await createInstallation();
  try {
    await setup(root);
    assert.equal(await gitSetting(root, 'core.hooksPath'), HOOKS_DIRECTORY);

    // A second run leaves the setting where it is: it names the tracked directory rather than
    // accumulating a value per run.
    await setup(root);
    assert.equal(await gitSetting(root, 'core.hooksPath'), HOOKS_DIRECTORY);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test('setup refuses a directory that is not a working copy, before it writes anything', async () => {
  const root = await createCopy();
  try {
    await assert.rejects(setup(root), NOT_A_WORKING_COPY_PATTERN);
    await assert.rejects(readFile(join(root, ENVIRONMENT_FILE), 'utf8'));
    await assert.rejects(readFile(secretPath(root, 'db_admin_password'), 'utf8'));
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

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

    // The password of a role that did not exist when the installation was set up is created rather
    // than demanded: a legacy installation has no backup holding it.
    const databaseValues = await readGeneratedSecrets(root, DATABASE_SECRET_NAMES);
    assert.equal(new Set(databaseValues).size, DATABASE_SECRET_NAMES.length);
    for (const value of databaseValues) assert.match(value, SECRET_VALUE_PATTERN);

    await unlink(secretPath(root, 'simulator_token'));
    await assert.rejects(setup(root), MISSING_SECRET_PATTERN);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test('setup refuses a legacy installation that lost one of its own passwords', async () => {
  const root = await createInstallation();
  try {
    await mkdir(join(root, SECRETS_DIRECTORY));
    await writeFile(join(root, ENVIRONMENT_FILE), EXISTING_ENVIRONMENT);
    await writeSecrets(root, {
      db_admin_password: newSecretValue(),
      db_app_password: newSecretValue(),
      [LEGACY_SECRET]: newSecretValue(),
    });
    for (const name of LEGACY_DATABASE_SECRET_NAMES) {
      assert.match(await readFile(secretPath(root, name), 'utf8'), SECRET_VALUE_PATTERN);
    }

    await unlink(secretPath(root, 'db_app_password'));
    await assert.rejects(setup(root), MISSING_SECRET_PATTERN);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test('setup gives a current installation the password of a role added after it', async () => {
  const root = await createInstallation();
  try {
    await mkdir(join(root, SECRETS_DIRECTORY));
    await writeFile(join(root, ENVIRONMENT_FILE), EXISTING_ENVIRONMENT);
    // An installation that holds every capability but none of the password of the mail role, which
    // did not exist when it was set up: it is migrated rather than sent back to a backup.
    await writeSecrets(root, {
      db_admin_password: newSecretValue(),
      db_app_password: newSecretValue(),
    });
    for (const name of CAPABILITY_SECRETS) {
      await writeFile(secretPath(root, name), newSecretValue());
    }

    await setup(root);
    assert.match(await readFile(secretPath(root, 'mailstub_app_password'), 'utf8'), SECRET_VALUE_PATTERN);

    await unlink(secretPath(root, 'demo_control_token'));
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
    const firstPasswords = await readGeneratedSecrets(root, DATABASE_SECRET_NAMES);
    assert.equal(new Set(firstPasswords).size, DATABASE_SECRET_NAMES.length);

    await setup(root);
    const repeatedValues = await readGeneratedSecrets(root, CAPABILITY_SECRETS);
    // Only the comparison result is reported, so a failure cannot print credentials into a test artifact.
    assert.ok(
      firstValues.every((value, index) => value === repeatedValues[index]),
      'credentials changed',
    );
    const repeatedPasswords = await readGeneratedSecrets(root, DATABASE_SECRET_NAMES);
    assert.ok(
      firstPasswords.every((value, index) => value === repeatedPasswords[index]),
      'passwords changed',
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
