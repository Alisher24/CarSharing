// What an archive claims about itself, and what the map needs it to claim. The bytes below are the
// header of the upstream build the manifest pins, so the reading is checked against the format itself
// rather than against a fixture this code wrote for itself.

import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { HEADER_BYTES, checkArchive, parseHeader } from './header.mjs';

/**
 * The first 127 bytes of `https://build.protomaps.com/20260918.pmtiles`, as a hex string. It is the
 * whole header of a real archive: spec version 3, a clustered vector tile pyramid from zoom 0 to zoom
 * 15, and the bounds the format writes for the whole world.
 */
const REAL_HEADER =
  '504d54696c6573037f00000000000000b53c000000000000f7888511200000009b04000000000000928d85112000000' +
  '0e683fa14000000000040000000000000f74885112000000055555555000000008610980a000000007eaa180800000000' +
  '01020201000f002eb694493a4ecd00d2496bb7c5b132000000000000000000';

/** What the map is drawn from: the demonstration area of Bishkek, at the declared depth. */
const REQUIRED = { bbox: [74.55, 42.84, 74.65, 42.9], maxzoom: 15 };

/** The world bounds the format writes: the whole longitude span and the Mercator latitude limit. */
const WORLD = { west: -180, south: -85.0511287, east: 180, north: 85.0511287 };

/** The header of that build, as bytes. */
function realHeader() {
  return Uint8Array.from(Buffer.from(REAL_HEADER, 'hex'));
}

describe('reading the header of an archive', () => {
  test('reads the version, the zooms, the bounds and the tile type of a real archive', () => {
    assert.equal(realHeader().length, HEADER_BYTES);

    const header = parseHeader(realHeader());

    assert.equal(header.specVersion, 3);
    assert.equal(header.minZoom, 0);
    assert.equal(header.maxZoom, 15);
    assert.equal(header.tileType, 1);
    assert.equal(header.clustered, true);
    assert.deepEqual(header.bounds, WORLD);
  });

  test('refuses a file that does not begin with the magic bytes', () => {
    const bytes = realHeader();
    bytes[0] = 'X'.charCodeAt(0);

    assert.throws(() => parseHeader(bytes), /does not begin with PMTiles/);
  });

  test('refuses a file too short to hold a header', () => {
    assert.throws(() => parseHeader(realHeader().subarray(0, HEADER_BYTES - 1)), /127 bytes/);
  });

  test('refuses an archive that is not the revision this reader knows', () => {
    const bytes = realHeader();
    bytes[7] = 2;

    assert.throws(() => parseHeader(bytes), /only v3 is read/);
  });
});

describe('checking an archive against what the map needs', () => {
  test('accepts an archive that covers the declared ground', () => {
    assert.equal(checkArchive(parseHeader(realHeader()), REQUIRED), undefined);
  });

  test('names the edge an archive stops short of', () => {
    for (const edge of ['west', 'south', 'east', 'north']) {
      const clipped = clippedAt(parseHeader(realHeader()), edge);

      assert.equal(
        checkArchive(clipped, REQUIRED),
        `the archive does not reach the declared bbox to the ${edge}`,
        `an archive short to the ${edge} was accepted`,
      );
    }
  });

  test('refuses an archive that stops above the declared depth', () => {
    const header = { ...parseHeader(realHeader()), maxZoom: 14 };

    assert.equal(checkArchive(header, REQUIRED), 'the archive stops at zoom 14, and 15 is declared');
  });

  test('refuses an archive that is not drawn from vector tiles', () => {
    const header = { ...parseHeader(realHeader()), tileType: 2 };

    assert.equal(
      checkArchive(header, REQUIRED),
      'the archive holds tile type 2, and the map is drawn from vector tiles',
    );
  });

  test('refuses an archive that begins above the world view', () => {
    const header = { ...parseHeader(realHeader()), minZoom: 5 };

    assert.equal(
      checkArchive(header, REQUIRED),
      'the archive begins at zoom 5, and the map is drawn from the world view down',
    );
  });
});

/**
 * The same archive with one edge drawn inside the declared bbox, which is what a build cut from a
 * smaller bbox would produce.
 */
function clippedAt(header, edge) {
  const bounds = { ...header.bounds };
  if (edge === 'west') bounds.west = REQUIRED.bbox[0] + 0.01;
  if (edge === 'south') bounds.south = REQUIRED.bbox[1] + 0.01;
  if (edge === 'east') bounds.east = REQUIRED.bbox[2] - 0.01;
  if (edge === 'north') bounds.north = REQUIRED.bbox[3] - 0.01;
  return { ...header, bounds };
}
