// The cursors of a paginated collection, as a check has to be able to make and to read them. Every
// collection of this application signs its cursors with one key and one payload shape, so the checks
// of all of them share these helpers rather than each spelling the format again.
import { createHmac } from 'node:crypto';
import { readFileSync } from 'node:fs';

/** The file the stack mounts the cursor signing key from, which a check signs a cursor with. */
const CURSOR_KEY_FILE = '.secrets/cursor_hmac_key';

/**
 * How many characters of a cursor are its signature: SHA-256 without padding, in the alphabet the
 * contract declares. The rest of the cursor is the payload it covers.
 */
const SIGNATURE_LENGTH = 43;

/** The alphabet the contract declares for a cursor, which a page link has to carry unescaped. */
export const CURSOR_ALPHABET = /^[A-Za-z0-9_-]+$/;

/**
 * issueCursor signs a cursor the way the service does, for the checks that present one the running
 * API never issued: another operation, another owner, or another set of parameters. The key is the
 * one the stack mounted, so a cursor this helper signs is accepted unless the rule under test is the
 * one that refuses it.
 */
export function issueCursor(scope, { createdAt, id }) {
  return signPayload(
    JSON.stringify({
      v: 1,
      t: createdAt,
      id,
      op: scope.operation,
      sub: scope.owner,
      q: scope.params ?? {},
    }),
  );
}

/** Edits one character of a cursor's payload, leaving the signature it was issued with. */
export function tamper(cursor) {
  return cursor.slice(0, -1) + (cursor.endsWith('A') ? 'B' : 'A');
}

/** The position a cursor names, which a check signs again under the scope it presents it with. */
export function positionOf(cursor) {
  const payload = JSON.parse(Buffer.from(cursor.slice(SIGNATURE_LENGTH), 'base64url').toString('utf8'));
  return { createdAt: payload.t, id: payload.id };
}

function signPayload(payload) {
  const encoded = Buffer.from(payload).toString('base64url');
  const signature = createHmac('sha256', readCursorKey()).update(encoded).digest('base64url');
  return `${signature}${encoded}`;
}

/** The signing key the stack mounted, read from the file local setup wrote. */
function readCursorKey() {
  return readFileSync(CURSOR_KEY_FILE, 'utf8').trim();
}
