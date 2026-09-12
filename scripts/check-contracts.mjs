import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { readFile, readdir } from 'node:fs/promises';
import { dirname, join, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
async function generatedFiles() {
  const result = new Map();
  async function walk(path) {
    for (const entry of await readdir(path, { withFileTypes: true })) {
      const file = join(path, entry.name);
      if (entry.isDirectory()) await walk(file);
      else result.set(relative(root, file), createHash('sha256').update(await readFile(file)).digest('hex'));
    }
  }
  for (const name of ['publicapi', 'healthapi', 'internalapi', 'mailstubapi']) {
    await walk(join(root, 'backend/internal/contracts', name));
  }
  await walk(join(root, 'frontend/src/shared/api/generated'));
  return [...result].sort(([a], [b]) => a.localeCompare(b));
}
function run(command, args, cwd = root) {
  execFileSync(command, args, { cwd, stdio: 'inherit' });
}

const before = await generatedFiles();
run(process.execPath, ['scripts/generate-contracts.mjs']);
assert.deepEqual(await generatedFiles(), before, 'Generated files were stale; review and commit regenerated files.');
run('go', ['test', './...'], join(root, 'backend'));
run('go', ['vet', './...'], join(root, 'backend'));
run(process.execPath, ['--test', 'scripts/setup.test.mjs']);
// npm supplies its CLI path on Windows and Unix when invoked through the documented npm script.
assert.ok(process.env.npm_execpath, 'Run: npm --prefix tools/openapi run check');
run(process.execPath, [process.env.npm_execpath, 'run', 'build'], join(root, 'frontend'));
console.log('PASS: deterministic generation, all contracts/examples, Go tests/vet, setup and frontend build.');
