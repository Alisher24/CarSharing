import { randomBytes } from 'node:crypto';
import { mkdir, readFile, writeFile, access, chmod } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

async function exists(path) {
  try { await access(path); return true; }
  catch (error) { if (error.code === 'ENOENT') return false; throw error; }
}

export async function setup(root) {
  const directory = resolve(root, '.secrets');
  const envPath = resolve(root, '.env');
  const names = ['db_admin_password', 'db_app_password', 'simulator_token', 'demo_control_token',
    'mailstub_delivery_token', 'mailstub_demo_token', 'cursor_hmac_key', 'mailstub_cursor_hmac_key'];
  const existing = new Set();
  for (const name of names) {
    if (await exists(resolve(directory, name))) existing.add(name);
  }
  const legacyPath = resolve(directory, 'internal_token');
  const migrateLegacy = await exists(legacyPath) && names.slice(2).every(name => !existing.has(name));
  const required = migrateLegacy ? ['db_admin_password', 'db_app_password', 'internal_token'] : names;
  if (await exists(envPath)) {
    for (const name of required) {
      if (!await exists(resolve(directory, name))) {
        throw new Error('Existing setup is missing secret files. Restore them from your local backup; passwords were not regenerated.');
      }
    }
  }
  // Validate existing credentials before writing any migration output.
  for (const name of [...existing, ...(migrateLegacy ? ['internal_token'] : [])]) {
    if (!/^[a-f0-9]{64}\n?$/.test(await readFile(resolve(directory, name), 'utf8'))) {
      throw new Error('A secret file is invalid; existing values were not overwritten.');
    }
  }
  const legacyValue = migrateLegacy ? await readFile(legacyPath, 'utf8') : null;
  await mkdir(directory, { recursive: true, mode: 0o700 });
  if (process.platform !== 'win32') await chmod(directory, 0o700);
  for (const name of names) {
    try {
      // Individual bind-mounted secrets must be readable by non-root container users.
      // The private parent directory prevents other host users from accessing them.
      const value = name === 'simulator_token' && legacyValue !== null ? legacyValue : randomBytes(32).toString('hex') + '\n';
      await writeFile(resolve(directory, name), value, { flag: 'wx', mode: 0o444 });
    } catch (error) { if (error.code !== 'EEXIST') throw error; }
    if (!/^[a-f0-9]{64}\n?$/.test(await readFile(resolve(directory, name), 'utf8'))) {
      throw new Error('A secret file is invalid; existing values were not overwritten.');
    }
  }
  try {
    await writeFile(envPath, await readFile(resolve(root, '.env.example')), { flag: 'wx', mode: 0o600 });
  } catch (error) { if (error.code !== 'EEXIST') throw error; }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    await setup(resolve(dirname(fileURLToPath(import.meta.url)), '..'));
    console.log('Local configuration is ready. Existing secrets were preserved. Run: docker compose up --build -d');
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
