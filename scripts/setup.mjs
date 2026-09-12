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
const LEGACY_SECRET = 'internal_token';
const LEGACY_SUCCESSOR = 'simulator_token';

const SECRET_BYTES = 32;

// Every secret is SECRET_BYTES of randomness rendered as hex, with an optional trailing newline.
const HEX_SECRET = /^[a-f0-9]{64}\n?$/;

const MISSING_SECRETS_ERROR = 'Existing setup is missing secret files. '
  + 'Restore them from your local backup; passwords were not regenerated.';
const INVALID_SECRET_ERROR = 'A secret file is invalid; existing values were not overwritten.';

async function exists(path) {
  try { await access(path); return true; }
  catch (error) { if (error.code === 'ENOENT') return false; throw error; }
}

function newSecret() {
  return randomBytes(SECRET_BYTES).toString('hex') + '\n';
}

export async function setup(root) {
  const directory = resolve(root, '.secrets');
  const envPath = resolve(root, '.env');
  const existing = new Set();
  for (const name of SECRET_NAMES) {
    if (await exists(resolve(directory, name))) existing.add(name);
  }
  const legacyPath = resolve(directory, LEGACY_SECRET);
  const migrateLegacy = await exists(legacyPath)
    && CAPABILITY_SECRETS.every(name => !existing.has(name));
  const required = migrateLegacy ? [...DATABASE_SECRETS, LEGACY_SECRET] : SECRET_NAMES;
  if (await exists(envPath)) {
    for (const name of required) {
      if (!await exists(resolve(directory, name))) {
        throw new Error(MISSING_SECRETS_ERROR);
      }
    }
  }
  // Validate existing credentials before writing any migration output.
  for (const name of [...existing, ...(migrateLegacy ? [LEGACY_SECRET] : [])]) {
    if (!HEX_SECRET.test(await readFile(resolve(directory, name), 'utf8'))) {
      throw new Error(INVALID_SECRET_ERROR);
    }
  }
  const legacyValue = migrateLegacy ? await readFile(legacyPath, 'utf8') : null;
  await mkdir(directory, { recursive: true, mode: 0o700 });
  if (process.platform !== 'win32') await chmod(directory, 0o700);
  for (const name of SECRET_NAMES) {
    try {
      // Individual bind-mounted secrets must be readable by non-root container users.
      // The private parent directory prevents other host users from accessing them.
      const inheritsLegacy = name === LEGACY_SUCCESSOR && legacyValue !== null;
      const value = inheritsLegacy ? legacyValue : newSecret();
      await writeFile(resolve(directory, name), value, { flag: 'wx', mode: 0o444 });
    } catch (error) { if (error.code !== 'EEXIST') throw error; }
    if (!HEX_SECRET.test(await readFile(resolve(directory, name), 'utf8'))) {
      throw new Error(INVALID_SECRET_ERROR);
    }
  }
  try {
    await writeFile(envPath, await readFile(resolve(root, '.env.example')), { flag: 'wx', mode: 0o600 });
  } catch (error) { if (error.code !== 'EEXIST') throw error; }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    await setup(resolve(dirname(fileURLToPath(import.meta.url)), '..'));
    console.log('Local configuration is ready. Existing secrets were preserved.',
      'Run: docker compose up --build -d');
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
