import { createClient } from '@hey-api/openapi-ts';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

process.chdir(resolve(dirname(fileURLToPath(import.meta.url)), '../../frontend'));
await createClient({
  input: '../.tools/contracts/public.json',
  output: { path: 'src/shared/api/generated', tsConfigPath: resolve('tsconfig.json') },
  plugins: ['@hey-api/typescript', '@hey-api/sdk', '@hey-api/client-fetch'],
});
