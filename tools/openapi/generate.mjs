import { createClient } from '@hey-api/openapi-ts';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { browserClientContract, fromRepositoryRoot } from '../../scripts/contracts-manifest.mjs';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const frontendDirectory = resolve(repositoryRoot, 'frontend');

// The contract whose client this generates, and where the manifest says it is committed, so the
// directory is stated once for the generator and the staleness check both.
const { name, browserClient: clientDirectory } = await browserClientContract();
const documentPath = join(repositoryRoot, '.tools/contracts', `${name}.json`);

// The generator writes its output relative to the frontend package it belongs to, and resolves the
// tsconfig the output is compiled with from that package.
process.chdir(frontendDirectory);

await createClient({
  input: documentPath,
  output: {
    path: relative(frontendDirectory, fromRepositoryRoot(clientDirectory)),
    tsConfigPath: resolve('tsconfig.json'),
  },
  plugins: ['@hey-api/typescript', '@hey-api/sdk', '@hey-api/client-fetch'],
});
