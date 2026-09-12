import { execFileSync } from 'node:child_process';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

// The repository root owns the formatting toolchain, so the hook must run the exact Prettier the
// checks run, never a version resolved from a global installation.
const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const prettierEntryPoint = join(repositoryRoot, 'node_modules/prettier/bin/prettier.cjs');
const ignoreFile = join(repositoryRoot, '.prettierignore');

function git(args) {
  return execFileSync('git', args, { cwd: repositoryRoot, encoding: 'utf8' });
}

// Files retired from the index are excluded, so a deleted path is never reformatted.
function stagedFiles() {
  return git(['diff', '--cached', '--name-only', '--diff-filter=ACMR', '-z'])
    .split('\0')
    .filter((path) => path.length > 0);
}

function formatStagedFiles(files) {
  const prettierArguments = [prettierEntryPoint, '--write', '--ignore-unknown', '--ignore-path', ignoreFile, ...files];
  execFileSync(process.execPath, prettierArguments, { cwd: repositoryRoot, stdio: 'inherit' });
  execFileSync('git', ['add', '--', ...files], { cwd: repositoryRoot, stdio: 'inherit' });
}

function main() {
  const files = stagedFiles();
  if (files.length === 0) return;

  formatStagedFiles(files);
}

main();
