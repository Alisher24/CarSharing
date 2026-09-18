import { layers, namedFlavor } from '@protomaps/basemaps';
import type { LayerSpecification, StyleSpecification, SymbolLayerSpecification } from 'maplibre-gl';
import source from './basemap.json' with { type: 'json' };

/**
 * The one place that knows what lies under the map. Every address the map asks for is built here from
 * the declaration the image was built with, so the archive, the glyphs and the sprite cannot be asked
 * for from one address and served from another. Nothing here reaches beyond the installation: the
 * declaration names paths below one prefix, and the same server that answers this page answers them.
 */

/** What the caption under the map names, so the drawing is not passed off as something it is not. */
export const BASEMAP_CAPTION = source.caption;

/** The language the map is labelled in, which is the language of the interface. */
export const BASEMAP_LANGUAGE = source.lang;

/** How much of the drawn world the map opens on, taken from the ground the archive was cut to. */
export const BASEMAP_CENTRE: [number, number] = [
  (source.bbox[0] + source.bbox[2]) / 2,
  (source.bbox[1] + source.bbox[3]) / 2,
];

/** The name the vector source is given, which every layer of the basemap refers to. */
const BASEMAP_SOURCE = 'protomaps';

/** The marker standing for the file of a sprite, which is asked for whole. */
const SPRITE_SUFFIX = '{suffix}';

/** What a URL reader makes of the braces a marker is written with. */
const ESCAPED_OPEN_BRACE = '%7B';
const ESCAPED_CLOSE_BRACE = '%7D';

/**
 * Every address the map asks for, below the one prefix the declaration states.
 *
 * The archive is named without a leading slash, because the scheme it is read through takes the rest
 * of the address as its own path: `pmtiles://` followed by `/basemap/...` is `pmtiles:///basemap/...`,
 * a host that is empty and a path the reader never resolves. The three-slash address is accepted by
 * the style, so a map given one is drawn with no basemap and says nothing about why.
 *
 * @param origin the installation the map is served from, which the archive's address is completed with
 */
export function basemapUrls(origin: string) {
  const path = served(source.archivePath).replace(/^\//, '');
  return {
    archive: `pmtiles://${addressOf(path, origin)}`,
    glyphs: served(source.fontsPath),
    sprite: served(source.spritePath.replace(SPRITE_SUFFIX, '')),
  };
}

/**
 * One address of the map, as the whole address a style states. A style names its glyphs and its sprite
 * by address rather than by path, and the library refuses a path, so a path is completed with the
 * origin of the installation that serves it.
 *
 * The game of braces: reading an address escapes the braces a marker is written with, and the library
 * substitutes a font stack into the address it is given, so they are put back. An address that reached
 * the library escaped would ask the installation for a file literally named `%7Brange%7D`.
 */
function addressOf(path: string, origin: string): string {
  return new URL(path, origin).href.replaceAll(ESCAPED_OPEN_BRACE, '{').replaceAll(ESCAPED_CLOSE_BRACE, '}');
}

/**
 * The style the map is drawn from: the upstream flavor, labelled in the declared language, with the
 * archive as its only source. It is built rather than copied, so the language and the addresses stand
 * in one place instead of in a document of several thousand lines nobody would read.
 *
 * @param origin the installation the map is served from, which its own addresses are relative to
 */
export function basemapStyle(origin: string): StyleSpecification {
  const urls = basemapUrls(origin);
  return {
    version: 8,
    glyphs: addressOf(urls.glyphs, origin),
    sprite: addressOf(urls.sprite, origin),
    sources: {
      [BASEMAP_SOURCE]: {
        type: 'vector',
        url: urls.archive,
        attribution: source.attribution,
      },
    },
    layers: basemapSheets(),
  };
}

/** The layers of the basemap, in the order they are drawn. */
export function basemapSheets(): LayerSpecification[] {
  return layers(BASEMAP_SOURCE, namedFlavor(source.flavor), { lang: source.lang });
}

/**
 * Every font stack the style asks for. A style names its fonts in its layers, so the declaration can
 * only be held to them by reading them back: a stack the style names and the declaration does not
 * serve is a map with no labels on it, and nothing else would say so.
 *
 * A `text-font` value may be an expression rather than a list of names, so what is read back is
 * everything the expression mentions that is a font stack at all. A conditional naming a fallback
 * names it in the same layer as the one that always applies.
 */
export function basemapStacks(): string[] {
  const declared = new Set(source.fonts.map((font) => font.stack));
  const named = new Set<string>();
  for (const layer of basemapSheets()) {
    for (const font of fontsOf(labelsOf(layer))) {
      if (declared.has(font)) named.add(font);
    }
  }
  return [...named].sort();
}

/** What one layer asks its labels to be drawn in, which only a layer carrying text has. */
function labelsOf(layer: LayerSpecification): unknown {
  return layer.type === 'symbol' ? (layer as SymbolLayerSpecification).layout?.['text-font'] : undefined;
}

/** The font stacks one `text-font` value names, which may be an expression rather than a list. */
function fontsOf(value: unknown): string[] {
  if (typeof value === 'string') return [value];
  if (!Array.isArray(value)) return [];
  return value.flatMap(fontsOf);
}

/**
 * One path of the declaration as the path it is served at. The two markers MapLibre fills in are left
 * in place: it substitutes the font stack and the range itself, and asks for the sprite by the path
 * the style states.
 */
function served(path: string): string {
  return `${source.prefix}/${path.replace(SPRITE_SUFFIX, '')}`;
}
