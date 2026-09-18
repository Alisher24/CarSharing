// What the map is drawn from, checked for the things a build would otherwise discover late: an
// address that points away from the installation, a range that leaves Cyrillic unlabelled, a prefix
// that is not the one the proxy serves. Nothing here reads the archive or the glyphs themselves —
// that is `verify.mjs`, which runs where they were written.

import assert from 'node:assert/strict';
import { join } from 'node:path';
import { describe, test } from 'node:test';
import { archiveDestination, assets, destination, readManifest, sprites } from './manifest.mjs';

const manifest = await readManifest();
const declared = assets(manifest);

/** Where the map is served from, which is where everything the declaration names is written. */
const SERVED_FROM = 'frontend/public';

/** The path the archive is written to, as the one path both this test and the build state. */
const ARCHIVE_WRITTEN = join(SERVED_FROM, 'basemap', manifest.archivePath);

/** The range Latin labels are drawn from, which every stack the style names must carry. */
const LATIN = '0-255';

/** The range Cyrillic labels are drawn from, which the interface's own language needs. */
const CYRILLIC = '1024-1279';

describe('what the map is drawn from', () => {
  test('is served under a path rather than an address of its own', () => {
    assert.match(manifest.prefix, /^\/[^/]/);
    assert.ok(!manifest.prefix.endsWith('/'), `the prefix ${manifest.prefix} ends with a slash`);
    assert.equal(archiveDestination(SERVED_FROM, manifest), ARCHIVE_WRITTEN);
    assert.ok(
      declared.every((asset) => asset.served.startsWith(`${manifest.prefix}/`)),
      'a glyph or sprite is served outside the prefix',
    );
  });

  test('is written under the name the file has, not the one a URL carries', () => {
    for (const asset of declared) {
      const path = destination(SERVED_FROM, asset);

      assert.ok(
        !path.includes('%20'),
        `${asset.served} would be written to ${path}, which is a directory a URL carries rather than one a file lives in`,
      );
      assert.ok(path.endsWith(asset.name), `${path} is not named ${asset.name}`);
    }
    assert.ok(
      !archiveDestination(SERVED_FROM, manifest).includes('%20'),
      'the archive is written to a directory a URL carries rather than one a file lives in',
    );
  });

  test('is cut from a pinned daily build of the declared ground and depth', () => {
    const [west, south, east, north] = manifest.bbox;

    assert.equal(manifest.bbox.length, 4);
    assert.ok(west < east, `the bbox spans ${west}..${east} across`);
    assert.ok(south < north, `the bbox spans ${south}..${north} up`);
    assert.match(manifest.archive, /^https:\/\/build\.protomaps\.com\/\d{8}\.pmtiles$/);
    assert.ok(Number.isInteger(manifest.maxzoom) && manifest.maxzoom > 0);
    assert.match(manifest.toolVersion, /^v\d+\.\d+\.\d+$/);
  });

  test('labels the map in the language the interface is written in', () => {
    assert.equal(manifest.lang, 'ru');
    assert.equal(manifest.flavor, 'light');
  });

  test('credits the data the map is drawn from', () => {
    assert.ok(
      manifest.attribution.includes('OpenStreetMap'),
      `the attribution ${manifest.attribution} names no source`,
    );
  });
});

describe('the glyphs and the sprite', () => {
  test('are each fetched from an address and served below the prefix', () => {
    assert.ok(declared.length > 0);

    for (const asset of declared) {
      assert.match(asset.address, /^https:\/\//, `${asset.served} is fetched from ${asset.address}`);
      assert.ok(asset.name.length > 0, `${asset.served} is written under no name`);
    }
  });

  test('carry Latin and Cyrillic for every stack the style names', () => {
    for (const font of manifest.fonts) {
      assert.ok(font.ranges.includes(LATIN), `${font.stack} carries no ${LATIN}`);
      assert.ok(font.ranges.includes(CYRILLIC), `${font.stack} carries no ${CYRILLIC}`);
    }
    assert.ok(
      declared.some((asset) => asset.served.includes(encodeURIComponent('Noto Sans Regular'))),
      'no glyph of the stack that labels most of the map is declared',
    );
  });

  test('hold the sprite at both densities, since a display may ask for either', () => {
    const served = sprites(manifest).map((sprite) => sprite.served);

    for (const suffix of ['.json', '.png', '@2x.json', '@2x.png']) {
      const path = `${manifest.prefix}/${manifest.spritePath.replace('{suffix}', suffix)}`;
      assert.ok(served.includes(path), `no ${path}`);
    }
  });

  test('are served at a path a URL can carry, with nothing left in it that a URL would escape', () => {
    for (const asset of declared) {
      assert.match(asset.served, /^[-A-Za-z0-9%._~!$&'()*+,;=:@/]+$/, `${asset.served} is not a URL path`);
    }
  });
});
