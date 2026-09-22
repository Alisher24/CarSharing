// The contract set, declared once for everything that projects or checks it: the generators, the
// staleness check and the Go tests all read this manifest rather than a list of their own. Adding a
// contract is an edit to `openapi/contracts.json` and to the two files it names for that contract.
import { readFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

/** Where the manifest lives, relative to the repository root, which every consumer resolves it from. */
export const MANIFEST_PATH = 'openapi/contracts.json';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

/**
 * One contract the repository maintains: the declaration the generators read.
 *
 * @typedef {object} Contract
 * @property {string} name the name `openapi/<name>.yaml` and `openapi/<name>.codegen.yaml` carry
 * @property {boolean} [servedByProduction] whether the production boundary registers its operations
 * @property {string} [browserClient] the committed directory a browser client is generated into
 * @property {string[]} [projections] the committed directories its Go projection is generated into
 */

/**
 * Every contract the manifest declares, in the order it declares them.
 *
 * @returns {Promise<Contract[]>} the contracts
 */
export async function contracts() {
  const manifest = JSON.parse(await readFile(join(repositoryRoot, MANIFEST_PATH), 'utf8'));
  if (!Array.isArray(manifest.contracts) || manifest.contracts.length === 0) {
    throw new Error(`${MANIFEST_PATH} declares no contract`);
  }
  return manifest.contracts;
}

/** The contract whose operations the production boundary registers, which exactly one declares. */
export async function servedContract() {
  const served = (await contracts()).filter((contract) => contract.servedByProduction);
  if (served.length !== 1) {
    throw new Error(`${MANIFEST_PATH} names ${served.length} contracts the production boundary serves`);
  }
  return served[0];
}

/**
 * The contract whose browser client this repository generates, which exactly one declares, and the
 * directory inside the frontend package it is written into.
 */
export async function browserClient() {
  const clients = (await contracts()).filter((contract) => contract.browserClient);
  if (clients.length !== 1) {
    throw new Error(`${MANIFEST_PATH} names ${clients.length} browser clients`);
  }
  return clients[0];
}

/** Every committed directory the projections of these contracts are written into. */
export async function generatedDirectories() {
  return (await contracts()).flatMap((contract) => [
    ...(contract.projections ?? []),
    ...(contract.browserClient ? [contract.browserClient] : []),
  ]);
}

/** Where a path the manifest states is, from the repository root every consumer works from. */
export function fromRepositoryRoot(path) {
  return join(repositoryRoot, path);
}
