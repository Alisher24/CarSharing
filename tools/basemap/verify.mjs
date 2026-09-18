// Checking the basemap against the declaration it was built from and against what was recorded while
// building it. An image build cuts the archive and fetches the glyphs, so the build fails here rather
// than at a map that is missing ground, labels or the picture beside them:
//
//   node tools/basemap/verify.mjs <the directory the map's prefix stands in> [what was recorded]
//
// Three things are checked and every one of them is reported: that the archive is the file that was
// cut, that its header claims the ground and the depth the declaration asks for, and that every glyph
// and sprite is the file that was fetched.

import { readFile } from 'node:fs/promises';
import { RECORD, archiveDestination, assets, destination, readManifest } from './manifest.mjs';
import { sha256 } from './files.mjs';
import { checkArchive, parseHeader } from './header.mjs';
import { readRecord } from './record.mjs';

const [root, record = RECORD] = process.argv.slice(2);
if (root === undefined) {
  process.stderr.write('usage: node tools/basemap/verify.mjs <where the prefix stands> [what was recorded]\n');
  process.exit(2);
}

const manifest = await readManifest();
const recorded = await readRecord(record);
const complaints = [];

/** Read one file, answering what went wrong instead of throwing, so every complaint is reported. */
async function read(path) {
  try {
    return await readFile(path);
  } catch (error) {
    complaints.push(`${path} could not be read: ${error.message}`);
    return undefined;
  }
}

async function checkArchiveFile() {
  const archive = recorded.archiveFile;
  if (archive === undefined) {
    complaints.push(`${record} names no archive; cut one with tools/basemap/extract.mjs`);
    return;
  }

  const bytes = await read(archiveDestination(root, manifest));
  if (bytes === undefined) return;

  if (bytes.length !== archive.bytes) {
    complaints.push(`the archive is ${bytes.length} bytes, and ${archive.bytes} was recorded`);
  }
  const digest = await sha256(bytes);
  if (digest !== archive.sha256) {
    complaints.push(`the archive digests to ${digest}, and ${archive.sha256} was recorded`);
  }

  const header = parseHeader(bytes);
  const wrong = checkArchive(header, { bbox: manifest.bbox, maxzoom: manifest.maxzoom });
  if (wrong === undefined) {
    process.stdout.write(
      `the archive covers ${header.bounds.west}..${header.bounds.east}, ` +
        `${header.bounds.south}..${header.bounds.north} at zoom ${header.minZoom}..${header.maxZoom}\n`,
    );
  } else {
    complaints.push(wrong);
  }
}

async function checkAssets() {
  const declared = assets(manifest);
  let checked = 0;
  for (const asset of declared) {
    const bytes = await read(destination(root, asset));
    if (bytes === undefined) continue;

    const digest = await sha256(bytes);
    if (digest !== recorded.checksums?.assets[asset.served]) {
      complaints.push(`${asset.served} digests to ${digest}, and ${record} records another file`);
      continue;
    }
    checked += 1;
  }
  process.stdout.write(`${checked} of ${declared.length} assets are the files that were fetched\n`);
}

await checkArchiveFile();
await checkAssets();

if (complaints.length > 0) {
  for (const complaint of complaints) process.stderr.write(`${complaint}\n`);
  process.exit(1);
}
process.stdout.write('PASS: the archive and every asset are what the declaration was built into.\n');
