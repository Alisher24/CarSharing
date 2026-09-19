// Fetching the glyphs and the sprite the style asks for, and recording what was fetched so the check
// that follows compares files rather than trusts them:
//
//   node tools/basemap/assets.mjs <the directory the map's prefix stands in> <what to record it in>
//
// It needs nothing but node, so it runs wherever the frontend is gathered.

import { RECORD, assets, destination, readManifest } from './manifest.mjs';
import { fetchBytes, sha256, writeBytes } from './files.mjs';
import { recordInto } from './record.mjs';

const [root, record = RECORD] = process.argv.slice(2);
if (root === undefined) {
  process.stderr.write('usage: node tools/basemap/assets.mjs <where the prefix stands> [what to record in]\n');
  process.exit(2);
}

const manifest = await readManifest();
const digests = {};
for (const asset of assets(manifest)) {
  const bytes = await fetchBytes(asset.address);
  await writeBytes(destination(root, asset), bytes);
  digests[asset.served] = await sha256(bytes);
  process.stdout.write(`fetched ${asset.served} (${bytes.length} bytes)\n`);
}

await recordInto(record, { checksums: { assets: digests } });
process.stdout.write(`${Object.keys(digests).length} assets recorded in ${record}\n`);
