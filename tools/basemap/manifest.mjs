// Reading what the map is drawn from, so that everything which needs to know agrees on it. The
// declaration states each address once; this module is the reading of it, and the frontend, the
// stages that build the basemap and the checks that hold them together all derive what they serve,
// fetch and check from here rather than from a list of their own.

import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

/** The declaration: what the map is drawn from, and where each part of it is served. */
export const MANIFEST = fileURLToPath(new URL('../../frontend/src/features/map/basemap.json', import.meta.url));

/** Where the repository keeps what a build records, when a build is run from the repository. */
export const RECORDS = fileURLToPath(new URL('../../.tools/basemap', import.meta.url));

/**
 * One file the map is drawn from besides the archive: the path it is served at, the name it is
 * written under, and the address it comes from. A font stack carries spaces in its name and `%20` in
 * the URL a browser asks for, so the two are stated separately rather than one being derived from the
 * other twice.
 *
 * @typedef {object} Asset
 * @property {string} served the whole path it is served at, the prefix included
 * @property {string} name the file name it is written under
 * @property {string} address the whole address it is fetched from
 */

/**
 * @typedef {object} Manifest
 * @property {string} prefix the path everything the map needs is served under
 * @property {string} archivePath where the archive is served, below that prefix
 * @property {string[]} spriteSuffixes the files of a sprite, by their ending
 * @property {string} fontsPath where a glyph is served, below that prefix, with its substitutions
 * @property {string} spritePath where a sprite file is served, below that prefix, with its substitution
 * @property {string} lang the language the style labels the map in
 * @property {string} flavor the upstream flavor the style is built from
 * @property {string} attribution what the map must credit for its data
 * @property {{stack: string, ranges: string[]}[]} fonts the glyphs, by stack and range
 * @property {string} sourceVersion the upstream basemap version the style and the tiles agree on
 * @property {string} archive the pinned upstream snapshot the archive is extracted from
 * @property {number[]} bbox the ground extracted from it, west south east north
 * @property {number} maxzoom the deepest level extracted
 * @property {string} toolVersion the extractor the archive is cut with
 * @property {string} glyphs the upstream address of a glyph, with its two substitutions
 * @property {string} sprite the upstream address of the sprite, without its extension
 */

/**
 * Read the declaration.
 *
 * @returns {Promise<Manifest>} what the map is drawn from
 */
export async function readManifest() {
  return JSON.parse(await readFile(MANIFEST, 'utf8'));
}

/** The path the archive is served at, which is one file at the top of the prefix. */
export function archiveServed(manifest) {
  return `${manifest.prefix}/${manifest.archivePath}`;
}

/**
 * Every glyph the declaration names, in the order it lists them.
 *
 * @param {Manifest} manifest what the map is drawn from
 * @returns {Asset[]} the glyphs
 */
export function glyphs(manifest) {
  return manifest.fonts.flatMap((font) =>
    font.ranges.map((range) => ({
      served: `${manifest.prefix}/${manifest.fontsPath}`
        .replace('{fontstack}', encodeURIComponent(font.stack))
        .replace('{range}', range),
      name: `${range}.pbf`,
      address: manifest.glyphs.replace('{fontstack}', encodeURIComponent(font.stack)).replace('{range}', range),
    })),
  );
}

/**
 * Every file of the sprite the declaration names, in the order it lists them. A sprite is an index
 * beside a picture, and both are kept at two densities, so a display with more pixels than its layout
 * asks for is drawn from a sharper picture rather than from a stretched one.
 *
 * The name a file is written under is the last part of the path it is served at, which is the whole
 * of what a sprite's name is: the declaration states it once, beside the address it comes from.
 *
 * @param {Manifest} manifest what the map is drawn from
 * @returns {Asset[]} the sprite files
 */
export function sprites(manifest) {
  return manifest.spriteSuffixes.map((suffix) => ({
    served: `${manifest.prefix}/${manifest.spritePath}`.replace('{suffix}', suffix),
    name: `${manifest.spritePath.replace('{suffix}', suffix).split('/').at(-1)}`,
    address: `${manifest.sprite}${suffix}`,
  }));
}

/**
 * Everything the map is drawn from besides the archive. A glyph and a sprite differ in where they come
 * from and nowhere else, so for fetching, writing and checking they are one kind of thing.
 *
 * @param {Manifest} manifest what the map is drawn from
 * @returns {Asset[]} the assets
 */
export function assets(manifest) {
  return [...glyphs(manifest), ...sprites(manifest)];
}

/**
 * Where one file served at this path, under this name, is written. The directory is taken from the
 * path itself, so a file lands in the directory it is served from without the two being stated twice,
 * and it is decoded: a browser asks for `%20` where the file on disk has a space.
 *
 * @param {string} root the directory the prefix stands in
 * @param {string} served the path it is served at
 * @param {string} name the file name it is written under
 * @returns {string} the path to write it at
 */
export function writtenAt(root, served, name) {
  const directory = served.slice(0, served.lastIndexOf('/'));
  return join(root, decodeURIComponent(directory), name);
}

/**
 * Where one asset is written.
 *
 * @param {string} root the directory the prefix stands in
 * @param {Asset} asset one glyph or sprite file
 * @returns {string} the path to write it at
 */
export function destination(root, asset) {
  return writtenAt(root, asset.served, asset.name);
}

/**
 * Where the archive is written.
 *
 * @param {string} root the directory the prefix stands in
 * @param {Manifest} manifest what the map is drawn from
 * @returns {string} the path to write it at
 */
export function archiveDestination(root, manifest) {
  return writtenAt(root, archiveServed(manifest), manifest.archivePath);
}

/** Where a build run from the repository records what it wrote. */
export const RECORD = join(RECORDS, 'record.json');
