import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { readFile, readdir } from 'node:fs/promises';
import { dirname, join, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const goBackendDirectory = join(repositoryRoot, 'backend');

// Generation is deterministic, so a digest of every committed output is enough to detect a stale
// one; the directories are the ones both generators write into.
const DIGEST_ALGORITHM = 'sha256';
const GENERATED_DIRECTORIES = [
  'backend/internal/contracts/publicapi',
  'backend/internal/contracts/servedapi',
  'backend/internal/contracts/internalapi',
  'backend/internal/contracts/mailstubapi',
  'frontend/src/shared/api/generated',
];

async function recordDigest(digests, path) {
  const contents = await readFile(path);
  digests.set(relative(repositoryRoot, path), createHash(DIGEST_ALGORITHM).update(contents).digest('hex'));
}

async function recordTreeDigests(digests, directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) await recordTreeDigests(digests, path);
    else await recordDigest(digests, path);
  }
}

async function generatedFileDigests() {
  const digests = new Map();
  for (const directory of GENERATED_DIRECTORIES) {
    await recordTreeDigests(digests, join(repositoryRoot, directory));
  }
  return [...digests].sort(([left], [right]) => left.localeCompare(right));
}

function run(command, args, workingDirectory = repositoryRoot) {
  execFileSync(command, args, { cwd: workingDirectory, stdio: 'inherit' });
}

function runNode(args, workingDirectory = repositoryRoot) {
  run(process.execPath, args, workingDirectory);
}

function runGo(args) {
  run('go', args, goBackendDirectory);
}

const digestsBeforeGeneration = await generatedFileDigests();
runNode(['scripts/generate-contracts.mjs']);
assert.deepEqual(
  await generatedFileDigests(),
  digestsBeforeGeneration,
  'Generated files were stale; review and commit regenerated files.',
);
runGo(['test', './...']);
runGo(['vet', './...']);
runNode(['--test', 'scripts/setup.test.mjs']);
// npm supplies its CLI path on Windows and Unix when invoked through the documented npm script.
assert.ok(process.env.npm_execpath, 'Run: npm --prefix tools/openapi run check');
runNode([process.env.npm_execpath, 'run', 'build'], join(repositoryRoot, 'frontend'));
console.log('PASS: deterministic generation, all contracts/examples, Go tests/vet, setup and frontend build.');
