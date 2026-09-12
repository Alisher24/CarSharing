import { createClient } from '@hey-api/openapi-ts';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const frontendDirectory = resolve(repositoryRoot, 'frontend');
const publicContractPath = resolve(repositoryRoot, '.tools/contracts/public.json');
const generatedClientDirectory = 'src/shared/api/generated';

// The generator writes its output relative to the frontend package it belongs to, and resolves the
// tsconfig the output is compiled with from that package.
process.chdir(frontendDirectory);

await createClient({
  input: publicContractPath,
  output: { path: generatedClientDirectory, tsConfigPath: resolve('tsconfig.json') },
  plugins: ['@hey-api/typescript', '@hey-api/sdk', '@hey-api/client-fetch'],
});
