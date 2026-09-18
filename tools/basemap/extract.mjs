// Cutting the archive the map is drawn from, out of the pinned upstream snapshot, and recording what
// was cut. The snapshot is the whole planet — a hundred gigabytes of it — and what is kept is the
// demonstration area at the declared depth:
//
//   node tools/basemap/extract.mjs <the directory the map's prefix stands in> [what to record in]
//
// The declaration names the snapshot, the ground, the depth and the extractor; nothing here states an
// address or a version. The extractor is whatever `PMTILES_EXTRACTOR` names, so the image build runs
// the binary it copied into the stage and a person runs the container image of the same release, and
// both are the same command.

import { execFileSync } from 'node:child_process';
import { mkdir, readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { RECORD, archiveDestination, readManifest } from './manifest.mjs';
import { sha256 } from './files.mjs';
import { parseHeader } from './header.mjs';
import { recordInto } from './record.mjs';

/** What runs the extractor: the binary the build installs, or the release's own image. */
const EXTRACTOR = process.env.PMTILES_EXTRACTOR;

const [root, record = RECORD] = process.argv.slice(2);
if (root === undefined) {
  process.stderr.write('usage: node tools/basemap/extract.mjs <where the prefix stands> [what to record in]\n');
  process.exit(2);
}

const manifest = await readManifest();
const path = archiveDestination(root, manifest);
await mkdir(dirname(path), { recursive: true });

const bbox = manifest.bbox.join(',');
const cutting = [manifest.archive, path, `--bbox=${bbox}`, `--maxzoom=${manifest.maxzoom}`];
process.stdout.write(`cutting ${bbox} at zoom ${manifest.maxzoom} out of ${manifest.archive}\n`);

if (EXTRACTOR === undefined) {
  // The directory the archive goes in is mounted into the container, so it is stated whole: a path
  // relative to where this ran would be a volume name to Docker rather than a place on the disk.
  const destination = resolve(dirname(path));
  execFileSync(
    'docker',
    [
      'run',
      '--rm',
      '--volume',
      `${destination}:/out`,
      `protomaps/go-pmtiles:${manifest.toolVersion}`,
      'extract',
      manifest.archive,
      `/out/${manifest.archivePath}`,
      `--bbox=${bbox}`,
      `--maxzoom=${manifest.maxzoom}`,
    ],
    { stdio: 'inherit' },
  );
} else {
  execFileSync(EXTRACTOR, ['extract', ...cutting], { stdio: 'inherit' });
}

const bytes = await readFile(path);
const header = parseHeader(bytes);
process.stdout.write(
  `the archive covers ${header.bounds.west}..${header.bounds.east}, ` +
    `${header.bounds.south}..${header.bounds.north} at zoom ${header.minZoom}..${header.maxZoom}\n`,
);

await recordInto(record, {
  archiveFile: { path, bytes: bytes.length, sha256: await sha256(bytes) },
});
process.stdout.write(`recorded ${record}\n`);
