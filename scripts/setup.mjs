import { randomBytes } from 'node:crypto';
import { mkdir, readFile, writeFile, access, chmod } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

// The database passwords predate the capability credentials, so a legacy installation holds only
// these two alongside LEGACY_SECRET.
const DATABASE_SECRETS = ['db_admin_password', 'db_app_password'];

// One credential per capability, so that revoking or rotating one does not affect the others.
export const CAPABILITY_SECRETS = [
  'simulator_token',
  'demo_control_token',
  'mailstub_delivery_token',
  'mailstub_demo_token',
  'cursor_hmac_key',
  'mailstub_cursor_hmac_key',
];

const SECRET_NAMES = [...DATABASE_SECRETS, ...CAPABILITY_SECRETS];

// A single internal_token used to cover every capability; it becomes the simulator token so an
// existing installation keeps working after the split.
export const LEGACY_SECRET = 'internal_token';
const LEGACY_SUCCESSOR = 'simulator_token';

export const SECRET_BYTES = 32;
const SECRET_HEX_DIGITS = SECRET_BYTES * 2;

export const SECRETS_DIRECTORY = '.secrets';
export const ENVIRONMENT_FILE = '.env';
const ENVIRONMENT_TEMPLATE = '.env.example';

// Every secret is SECRET_BYTES of randomness rendered as hex, with an optional trailing newline.
export const SECRET_VALUE_PATTERN = new RegExp(`^[a-f0-9]{${SECRET_HEX_DIGITS}}\\n?$`);
const SECRET_TRAILING_NEWLINE = '\n';

// A secret file must be readable by the non-root container user that mounts it, while its private
// parent directory keeps other host users out.
const SECRETS_DIRECTORY_MODE = 0o700;
const SECRET_FILE_MODE = 0o444;
const ENVIRONMENT_FILE_MODE = 0o600;

const MISSING_SECRETS_ERROR =
  'Existing setup is missing secret files. ' + 'Restore them from your local backup; passwords were not regenerated.';
const INVALID_SECRET_ERROR = 'A secret file is invalid; existing values were not overwritten.';

function secretPath(secretsDirectory, name) {
  return resolve(secretsDirectory, name);
}

function fileIsMissing(error) {
  return error.code === 'ENOENT';
}

async function exists(path) {
  try {
    await access(path);
    return true;
  } catch (error) {
    if (fileIsMissing(error)) return false;
    throw error;
  }
}

async function readSecretValue(secretsDirectory, name) {
  const path = secretPath(secretsDirectory, name);
  if (!(await exists(path))) return null;
  const value = await readFile(path, 'utf8');
  if (!SECRET_VALUE_PATTERN.test(value)) throw new Error(INVALID_SECRET_ERROR);
  return value;
}

function newSecretValue() {
  return randomBytes(SECRET_BYTES).toString('hex') + SECRET_TRAILING_NEWLINE;
}

function fileAlreadyExists(error) {
  return error.code === 'EEXIST';
}

async function writeNewSecret(secretsDirectory, name, value) {
  try {
    await writeFile(secretPath(secretsDirectory, name), value, { flag: 'wx', mode: SECRET_FILE_MODE });
  } catch (error) {
    if (!fileAlreadyExists(error)) throw error;
  }
}

/**
 * Reports the credentials a legacy installation must already hold before it can migrate: every
 * capability credential, or the pair of database passwords and the single internal token that
 * predates them.
 */
async function requiredSecretNames(secretsDirectory) {
  const capabilities = await Promise.all(CAPABILITY_SECRETS.map((name) => exists(secretPath(secretsDirectory, name))));
  if (capabilities.some(Boolean)) return SECRET_NAMES;
  const hasLegacySecret = await exists(secretPath(secretsDirectory, LEGACY_SECRET));
  return hasLegacySecret ? [...DATABASE_SECRETS, LEGACY_SECRET] : SECRET_NAMES;
}

async function requireExistingSecrets(secretsDirectory, names) {
  for (const name of names) {
    if (!(await exists(secretPath(secretsDirectory, name)))) throw new Error(MISSING_SECRETS_ERROR);
  }
}

/**
 * Reads back the credentials a previous run left, so they survive this one. The legacy token becomes
 * the simulator token, which is what keeps an existing installation working after the split.
 */
async function readPreservedSecrets(secretsDirectory) {
  const preserved = new Map();
  for (const name of SECRET_NAMES) {
    const value = await readSecretValue(secretsDirectory, name);
    if (value !== null) preserved.set(name, value);
  }
  const legacyValue = await readSecretValue(secretsDirectory, LEGACY_SECRET);
  if (legacyValue !== null && !preserved.has(LEGACY_SUCCESSOR)) {
    preserved.set(LEGACY_SUCCESSOR, legacyValue);
  }
  return preserved;
}

async function createSecretsDirectory(secretsDirectory) {
  await mkdir(secretsDirectory, { recursive: true, mode: SECRETS_DIRECTORY_MODE });
  // Windows has no POSIX mode bits, and mkdir above already applied the mode elsewhere.
  if (process.platform === 'win32') return;
  await chmod(secretsDirectory, SECRETS_DIRECTORY_MODE);
}

async function writeSecrets(secretsDirectory, preserved) {
  for (const name of SECRET_NAMES) {
    const value = preserved.get(name) ?? newSecretValue();
    await writeNewSecret(secretsDirectory, name, value);
    // A concurrent run may have won the write, so trust only what the file holds now.
    if (preserved.has(name)) continue;
    await readSecretValue(secretsDirectory, name);
  }
}

async function createEnvironmentFile(root) {
  const environmentPath = resolve(root, ENVIRONMENT_FILE);
  if (await exists(environmentPath)) return;
  const template = await readFile(resolve(root, ENVIRONMENT_TEMPLATE));
  await writeFile(environmentPath, template, { flag: 'wx', mode: ENVIRONMENT_FILE_MODE });
}

export async function setup(root) {
  const secretsDirectory = resolve(root, SECRETS_DIRECTORY);
  // A half-migrated installation must fail before this run writes anything of its own.
  if (await exists(resolve(root, ENVIRONMENT_FILE))) {
    await requireExistingSecrets(secretsDirectory, await requiredSecretNames(secretsDirectory));
  }
  const preserved = await readPreservedSecrets(secretsDirectory);
  await createSecretsDirectory(secretsDirectory);
  await writeSecrets(secretsDirectory, preserved);
  await createEnvironmentFile(root);
}

async function main() {
  await setup(resolve(dirname(fileURLToPath(import.meta.url)), '..'));
  console.log('Local configuration is ready. Existing secrets were preserved.', 'Run: docker compose up --build -d');
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    await main();
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
