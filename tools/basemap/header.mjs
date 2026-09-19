// The PMTiles v3 header, read back as the claim an archive makes about itself. The archive is built
// during an image build from a pinned upstream snapshot, so what it holds is checked against the
// declaration rather than assumed: a smaller bbox or a lower zoom would otherwise show itself as
// labels missing from the map instead of as a failed build.
//
// The layout below is the format's own and is fixed at 127 bytes, read from offset zero. Every
// number is little-endian, and the four bounds are degrees written as whole ten-millionths.

/** How many bytes of the archive the header occupies. */
export const HEADER_BYTES = 127;

/** The bytes every archive begins with, which is what makes it readable as one. */
export const MAGIC = 'PMTiles';

/** The only header revision this reader knows. */
export const SPEC_VERSION = 3;

/** The tile type of a vector archive: Mapbox Vector Tiles. */
export const TILE_TYPE_MVT = 1;

/** What one degree is worth in the whole numbers the header states a bound in. */
const DEGREES_HELD_AS = 1e7;

/** How far apart two readings of one bound may be and still count as the same place, in degrees. */
const BOUND_TOLERANCE = 1e-6;

/** Where each field of the header begins. */
const AT = {
  specVersion: 7,
  clustered: 96,
  tileType: 99,
  minZoom: 100,
  maxZoom: 101,
  west: 102,
  south: 106,
  east: 110,
  north: 114,
};

/**
 * parseHeader reads the header out of the first bytes of an archive and refuses anything that is not a
 * v3 PMTiles header. It reads the header alone: whether the archive covers the ground the map needs is
 * the separate question `checkArchive` answers.
 *
 * @param {Uint8Array} bytes the first bytes of an archive
 * @returns {{specVersion: number, minZoom: number, maxZoom: number, tileType: number, clustered: boolean,
 *   bounds: {west: number, south: number, east: number, north: number}}} what the archive claims
 * @throws {Error} when the bytes are not a v3 PMTiles header
 */
export function parseHeader(bytes) {
  if (bytes.length < HEADER_BYTES) {
    throw new Error(`an archive header is ${HEADER_BYTES} bytes, and this file holds ${bytes.length}`);
  }
  const magic = String.fromCharCode(...bytes.subarray(0, MAGIC.length));
  if (magic !== MAGIC) {
    throw new Error(`this file does not begin with ${MAGIC}, so it is not a PMTiles archive`);
  }

  const view = new DataView(bytes.buffer, bytes.byteOffset, HEADER_BYTES);
  const specVersion = view.getUint8(AT.specVersion);
  if (specVersion !== SPEC_VERSION) {
    throw new Error(`this archive is a v${specVersion} one, and only v${SPEC_VERSION} is read`);
  }

  return {
    specVersion,
    minZoom: view.getUint8(AT.minZoom),
    maxZoom: view.getUint8(AT.maxZoom),
    tileType: view.getUint8(AT.tileType),
    clustered: view.getUint8(AT.clustered) === 1,
    bounds: {
      west: degreesAt(view, AT.west),
      south: degreesAt(view, AT.south),
      east: degreesAt(view, AT.east),
      north: degreesAt(view, AT.north),
    },
  };
}

/**
 * checkArchive describes what an archive fails to be, or answers nothing when it is the archive the
 * map is served with. It answers the difference rather than a boolean, because naming the reason is
 * the whole value of the check.
 *
 * @param {object} header what an archive claims, as `parseHeader` reads it
 * @param {{bbox: number[], maxzoom: number}} required the ground and the zoom the map needs covered
 * @returns {string|undefined} the difference, or nothing when there is none
 */
export function checkArchive(header, required) {
  if (header.tileType !== TILE_TYPE_MVT) {
    return `the archive holds tile type ${header.tileType}, and the map is drawn from vector tiles`;
  }
  if (header.maxZoom !== required.maxzoom) {
    return `the archive stops at zoom ${header.maxZoom}, and ${required.maxzoom} is declared`;
  }
  if (header.minZoom > 0) {
    return `the archive begins at zoom ${header.minZoom}, and the map is drawn from the world view down`;
  }

  const [west, south, east, north] = required.bbox;
  const wanting = [
    header.bounds.west > west + BOUND_TOLERANCE ? 'west' : undefined,
    header.bounds.south > south + BOUND_TOLERANCE ? 'south' : undefined,
    header.bounds.east < east - BOUND_TOLERANCE ? 'east' : undefined,
    header.bounds.north < north - BOUND_TOLERANCE ? 'north' : undefined,
  ].filter((edge) => edge !== undefined);

  if (wanting.length > 0) {
    return `the archive does not reach the declared bbox to the ${wanting.join(', ')}`;
  }
  return undefined;
}

/** degreesAt reads one bound the way the header states it: ten-millionths of a degree, whole. */
function degreesAt(view, offset) {
  return view.getInt32(offset, true) / DEGREES_HELD_AS;
}
