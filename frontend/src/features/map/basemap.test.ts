import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';
import { basemapStacks, basemapStyle, basemapUrls } from './basemap.ts';

/** The declaration the image was built with, read as text because a test holds it to what it states. */
const DECLARATION = JSON.parse(readFileSync(new URL('./basemap.json', import.meta.url), 'utf8'));

/** The proxy configuration, which is the other half of every address the declaration states. */
const NGINX = readFileSync(new URL('../../../../infra/nginx.conf', import.meta.url), 'utf8');

/** The origin these checks serve the map from, which its addresses are completed with. */
const ORIGIN = 'http://127.0.0.1:8080';

/** The ranges a label may be drawn from: Latin, which is most of the map, and Cyrillic, which is this. */
const REQUIRED_RANGES = ['0-255', '1024-1279'];

describe('the addresses the map asks for', () => {
  test('are all below the prefix the proxy serves the basemap at', () => {
    const urls = basemapUrls(ORIGIN);

    for (const [what, address] of Object.entries(urls)) {
      assert.ok(address.includes(`${DECLARATION.prefix}/`), `${what} is asked for at ${address}`);
    }
  });

  test('name the archive with one slash after the scheme, not three', () => {
    // `pmtiles:///basemap/...` is an address with an empty host, and the reader leaves it alone: the
    // map is then drawn with no basemap and says nothing about why.
    assert.match(basemapUrls(ORIGIN).archive, /^pmtiles:\/\/[^/]/);
  });

  test('name the archive as a whole address, since it is read through a scheme of its own', () => {
    const { archive } = basemapUrls(ORIGIN);

    assert.equal(archive, `pmtiles://${ORIGIN}${DECLARATION.prefix}/${DECLARATION.archivePath}`);
    assert.doesNotThrow(() => new URL(archive.replace('pmtiles://', '')));
  });

  test('are the prefix the proxy answers with a location of its own', () => {
    assert.match(
      NGINX,
      new RegExp(`location\\s+${DECLARATION.prefix}/\\s*\\{`),
      `infra/nginx.conf has no location for ${DECLARATION.prefix}/`,
    );
  });

  test('reach no other host, since nothing of the map comes from outside the installation', () => {
    const { archive, glyphs, sprite } = basemapUrls(ORIGIN);

    assert.ok(archive.startsWith(`pmtiles://${ORIGIN}${DECLARATION.prefix}/`), archive);
    for (const path of [glyphs, sprite]) {
      assert.ok(path.startsWith(`${DECLARATION.prefix}/`), `the map asks for ${path}`);
      assert.ok(!path.includes('://'), `the map asks for ${path}`);
    }
  });

  test('leave the markers MapLibre fills in for itself, so a label can name its font stack', () => {
    const { glyphs } = basemapUrls(ORIGIN);

    assert.ok(glyphs.includes('{fontstack}') && glyphs.includes('{range}'), glyphs);
    assert.ok(!glyphs.includes(' '), `${glyphs} carries a character a URL may not`);
  });
});

describe('the style the map is drawn from', () => {
  test('is built from the archive the declaration names, below that same prefix', () => {
    const style = basemapStyle(ORIGIN);

    assert.equal(style.version, 8);
    assert.equal(style.sources.protomaps.type, 'vector');
    assert.equal(style.sources.protomaps.url, basemapUrls(ORIGIN).archive);
    assert.ok(style.layers.length > 1);
  });

  test('names its glyphs and its sprite by whole addresses of this installation', () => {
    const style = basemapStyle(ORIGIN);

    // The library refuses a path here, so a style that states one draws a map with no labels on it
    // and says nothing about why.
    assert.equal(style.glyphs, `${ORIGIN}${basemapUrls(ORIGIN).glyphs}`);
    assert.equal(style.sprite, `${ORIGIN}${basemapUrls(ORIGIN).sprite}`);
  });

  test('keeps the markers MapLibre fills in, so a label can name its font stack', () => {
    const { glyphs } = basemapStyle(ORIGIN);

    assert.ok(String(glyphs).includes('{fontstack}') && String(glyphs).includes('{range}'), String(glyphs));
  });

  test('labels the map in the language the interface is written in', () => {
    assert.equal(DECLARATION.lang, 'ru');
  });

  test('credits the data it is drawn from, on the map itself', () => {
    const style = basemapStyle(ORIGIN).sources.protomaps;

    assert.equal(style.type, 'vector');
    assert.ok(
      style.type === 'vector' && String(style.attribution).includes('OpenStreetMap'),
      'the map names no source of its data',
    );
  });

  test('asks for no font stack the declaration does not serve', () => {
    const served = DECLARATION.fonts.map((font: { stack: string }) => font.stack);

    assert.ok(basemapStacks().length > 0, 'the style names no font at all');
    for (const stack of basemapStacks()) {
      assert.ok(served.includes(stack), `the style labels the map in ${stack}, which nothing serves`);
    }
  });

  test('serves every stack it names in the ranges its own language needs', () => {
    for (const font of DECLARATION.fonts as { stack: string; ranges: string[] }[]) {
      for (const range of REQUIRED_RANGES) {
        assert.ok(font.ranges.includes(range), `${font.stack} carries no ${range}`);
      }
    }
  });
});
