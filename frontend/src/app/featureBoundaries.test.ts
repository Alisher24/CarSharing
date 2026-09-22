import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, test } from 'node:test';
import { fileURLToPath } from 'node:url';

/**
 * Where a module of one feature may be imported from, stated once and held by this check. `src/shared`
 * is what more than one feature uses, `src/app` is the shell that names the features and the addresses
 * they are reached at, and a feature is reachable only from itself: one feature importing another is
 * how a cycle, a second copy of a value and an internal that nobody meant to publish appear.
 */
const ALLOWED_TARGETS = ['shared', 'app'];

/** Where the application is, which is what the check walks. */
const SOURCE = fileURLToPath(new URL('..', import.meta.url)).replace(/[\\/]$/, '');

/** Every module the source tree is read from, whether it is imported or re-exported. */
const IMPORT = /(?:from|import)\s+'(\.[^']+)'/g;

/** Every module of the source tree, so the walk depends on no list that has to be kept in step. */
async function sourceFiles(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map((entry) => {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) return sourceFiles(path);
      return entry.name.endsWith('.ts') || entry.name.endsWith('.tsx') ? [path] : [];
    }),
  );

  return files.flat();
}

/**
 * The module one relative import names, as the segments below `src`, or nothing when the specifier
 * leaves `src` or names a module of one feature from another. The feature of the importing module is
 * what the answer is compared against, so `./copy.ts` is a module of its own feature and
 * `../other/copy.ts` is a crossing.
 */
function targetOf(file: string, specifier: string): string[] | undefined {
  const directory = file
    .slice(SOURCE.length + 1)
    .split(/[\\/]/)
    .slice(0, -1);
  const resolved = specifier.split('/').reduce<string[]>((kept, segment) => {
    if (segment === '.') return kept;
    if (segment === '..') return kept.slice(0, -1);
    return [...kept, segment];
  }, directory);

  return resolved[0] === 'shared' || resolved[0] === 'app' || resolved[0] === 'features' ? resolved : undefined;
}

/** The feature one module belongs to, or nothing when it is not a feature's own module. */
function featureOf(path: readonly string[]): string | undefined {
  return path[0] === 'features' ? path[1] : undefined;
}

/** One import of a feature's internal from another feature. */
type Crossing = { file: string; specifier: string };

describe('the boundary between features', () => {
  test('is crossed by no import of one feature from another', async () => {
    const crossings: Crossing[] = [];

    for (const file of await sourceFiles(SOURCE)) {
      const name = file.slice(SOURCE.length + 1);
      const owner = featureOf(name.split(/[\\/]/));
      if (owner === undefined) continue;

      const source = await readFile(file, 'utf8');
      for (const [, specifier] of source.matchAll(IMPORT)) {
        const target = targetOf(file, specifier);
        if (target === undefined) continue;

        const imported = featureOf(target);
        // The shell names every feature it shows, which is the one place a feature is reached from.
        if (imported === undefined || imported === owner || owner === 'app') continue;

        crossings.push({ file: name, specifier });
      }
    }

    assert.deepEqual(
      crossings,
      [],
      `a feature may import only itself, ${ALLOWED_TARGETS.join(' or ')}; move what is shared into src/shared`,
    );
  });
});
