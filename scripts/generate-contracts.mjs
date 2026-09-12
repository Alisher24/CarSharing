import { execFileSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const backend = resolve(root, 'backend');
function go(...args) {
  execFileSync('go', args, { cwd: backend, stdio: 'inherit' });
}
for (const name of ['public', 'internal', 'mailstub']) {
  go('run', './cmd/contracts', `../openapi/${name}.yaml`, `../.tools/contracts/${name}.json`);
  go('tool', 'oapi-codegen', '--config', `../openapi/${name}.codegen.yaml`, `../.tools/contracts/${name}.json`);
}
// Derive the served interface from the same public source so planned operations cannot be registered accidentally.
go('tool', 'oapi-codegen', '--config', '../openapi/served.codegen.yaml', '../.tools/contracts/public.json');
execFileSync(process.execPath, [resolve(root, 'tools/openapi/generate.mjs')], { cwd: root, stdio: 'inherit' });
