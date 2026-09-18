// Recording what a stage of the basemap build wrote, so the stage that follows checks files rather
// than trusts them. Each stage records the part it owns and leaves the rest alone: the archive and the
// assets are cut and fetched by different stages, and neither knows about the other's work.
//
// The record is written outside the directory the map is served from. A record kept beside the archive
// would be copied into the built site and published with it, and what a build measured is nobody's
// business but the build's.

import { readFile } from 'node:fs/promises';
import { writeBytes } from './files.mjs';

/** What the build recorded, in the parts its stages own. */
const PARTS = ['archiveFile', 'checksums'];

/**
 * @typedef {object} Record
 * @property {{path: string, bytes: number, sha256: string}} [archiveFile] the archive as it was cut
 * @property {{assets: Record<string, string>}} [checksums] the digest of every asset, by served path
 */

/**
 * Read what the build has recorded so far. A record that is not there yet is an empty one, because the
 * stage that reads it runs either side of the stage that writes it.
 *
 * @param {string} path where the record is
 * @returns {Promise<Record>} the record
 */
export async function readRecord(path) {
  try {
    return JSON.parse(await readFile(path, 'utf8'));
  } catch {
    return {};
  }
}

/**
 * Add one part to the record and write it back, leaving the parts another stage owns as they are. The
 * directory is created if it is not there, because a stage is given a path rather than a directory and
 * nothing else in the build makes one for it.
 *
 * @param {string} path where the record is
 * @param {Record} part what this stage produced
 */
export async function recordInto(path, part) {
  const kept = await readRecord(path);
  const merged = {};
  for (const name of PARTS) {
    if (part[name] !== undefined) merged[name] = part[name];
    else if (kept[name] !== undefined) merged[name] = kept[name];
  }
  await writeBytes(path, new TextEncoder().encode(`${JSON.stringify(merged, null, 2)}\n`));
}
