// Reading one file from an address and holding it where it belongs, which is all the basemap build
// needs. A single glyph and a hundred-megabyte archive arrive the same way.

import { mkdir, writeFile } from 'node:fs/promises';
import { dirname } from 'node:path';

/**
 * fetchBytes reads one address into memory, refusing anything but a complete answer. A partial
 * download is worse than a missing one: it would be written out and only show itself much later, as a
 * map that renders half a city.
 *
 * @param {string} address what to read
 * @returns {Promise<Uint8Array>} the whole answer
 */
export async function fetchBytes(address) {
  const answer = await fetch(address, { redirect: 'follow' });
  if (!answer.ok) {
    throw new Error(`${answer.status} ${answer.statusText} for ${address}`);
  }
  return new Uint8Array(await answer.arrayBuffer());
}

/**
 * writeBytes writes one file, creating the directories it needs.
 *
 * @param {string} path where to write
 * @param {Uint8Array} bytes what to write
 */
export async function writeBytes(path, bytes) {
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, bytes);
}

/**
 * sha256 is the digest of what was read, in lowercase hexadecimal. It is what makes a downloaded file
 * checkable on the next build rather than merely present.
 *
 * @param {Uint8Array} bytes what to digest
 * @returns {Promise<string>} the digest
 */
export async function sha256(bytes) {
  const digest = await crypto.subtle.digest('SHA-256', bytes);
  return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, '0')).join('');
}
