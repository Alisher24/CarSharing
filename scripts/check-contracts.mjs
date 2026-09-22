// The staleness check of the committed contract projections: every generated directory the manifest
// declares is digested, the whole pipeline is run again, and a difference means the committed tree is
// not what the sources produce. Regenerating leaves the new files in place for review rather than
// reverting them, so a failure that says "stale" is an instruction to look at the diff.
//
// Nothing else is checked here. The Go checks run in the backend job, the frontend build in the
// frontend job, and the declarations the repository holds in step in `npm test`; a check that runs
// twice is a check whose failure has two places to be read in.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { readFile, readdir } from 'node:fs/promises';
import { dirname, join, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { contracts, fromRepositoryRoot, generatedDirectories } from './contracts-manifest.mjs';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

// Generation is deterministic, so a digest of every committed output is enough to detect a stale one.
const DIGEST_ALGORITHM = 'sha256';

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
  for (const directory of await generatedDirectories()) {
    await recordTreeDigests(digests, fromRepositoryRoot(directory));
  }
  return [...digests].sort(([left], [right]) => left.localeCompare(right));
}

// The Go projection a contract declares is the one its generator configuration names, so a manifest
// that stopped covering it would leave a committed tree nobody regenerates.
async function checkProjectionsAreGenerated() {
  for (const contract of await contracts()) {
    const configuration = await readFile(fromRepositoryRoot(`openapi/${contract.name}.codegen.yaml`), 'utf8');
    const declared = configuration.match(/^output: (.+)$/m);
    assert.ok(declared, `openapi/${contract.name}.codegen.yaml names no output`);
    const directory = dirname(declared[1]);
    assert.ok(
      (contract.projections ?? []).includes(`backend/${directory}`),
      `${contract.name} is generated into backend/${directory}, which the manifest does not project`,
    );
  }
}

const digestsBeforeGeneration = await generatedFileDigests();
await checkProjectionsAreGenerated();
execFileSync(process.execPath, [join(repositoryRoot, 'scripts/generate-contracts.mjs')], {
  cwd: repositoryRoot,
  stdio: 'inherit',
});
assert.deepEqual(
  await generatedFileDigests(),
  digestsBeforeGeneration,
  'Generated files were stale; review and commit regenerated files.',
);
console.log('PASS: deterministic generation of every contract the manifest declares.');
