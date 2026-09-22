import { execFileSync } from 'node:child_process';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { contracts, servedContract } from './contracts-manifest.mjs';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const backendDirectory = join(repositoryRoot, 'backend');
const contractToolsDirectory = join(repositoryRoot, '.tools/contracts');
const openapiDirectory = join(repositoryRoot, 'openapi');

function runGo(args) {
  execFileSync('go', args, { cwd: backendDirectory, stdio: 'inherit' });
}

function generateGoContract(name) {
  const sourcePath = join(openapiDirectory, `${name}.yaml`);
  const documentPath = join(contractToolsDirectory, `${name}.json`);
  const configPath = join(openapiDirectory, `${name}.codegen.yaml`);

  runGo(['run', './cmd/contracts', sourcePath, documentPath]);
  runGo(['tool', 'oapi-codegen', '--config', configPath, documentPath]);
}

for (const { name } of await contracts()) {
  generateGoContract(name);
}
// Derive the served interface from the source of the contract the production boundary serves, so a
// planned operation cannot be registered accidentally.
const served = await servedContract();
runGo([
  'tool',
  'oapi-codegen',
  '--config',
  join(openapiDirectory, 'served.codegen.yaml'),
  join(contractToolsDirectory, `${served.name}.json`),
]);
execFileSync(process.execPath, [join(repositoryRoot, 'tools/openapi/generate.mjs')], {
  cwd: repositoryRoot,
  stdio: 'inherit',
});
