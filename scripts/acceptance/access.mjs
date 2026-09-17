// The access matrix: every operation the public contract serves, what it needs to be called, and who
// its answer belongs to. The list of operations and their paths are read from the contract sources,
// so an operation added to the served set fails the suite until it is classified here; only the
// classification itself is written by hand, because only a person knows whether an operation has an
// owner.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { repositoryRoot } from '../service.mjs';
import { call } from './client.mjs';

/** The contract source the served set is generated from. */
const SERVED_CONFIG_PATH = join(repositoryRoot, 'openapi/served.codegen.yaml');

/** The contract source every served operation is declared in. */
const PUBLIC_CONTRACT_PATH = join(repositoryRoot, 'openapi/public.yaml');

/** A path segment of the contract's templated form, which a request replaces with an identifier. */
const TEMPLATE_SEGMENT = /\{[^}]+\}/;

/**
 * The identifier kind every templated segment carries. Both are stated because the contract declares
 * them: a rental, an invoice and a notification are v7 identifiers, which is the version of the
 * contract's error envelope that carries a request identifier.
 */
export const ABSENT_IDENTIFIER = '00000000-0000-7000-8000-0000000000ff';
export const ABSENT_VEHICLE_IDENTIFIER = '00000000-0000-7000-8000-0000000000fe';

/**
 * What a caller must be to reach an operation. The distinction is the whole of the access rule:
 * an operation open to anyone cannot leak an account's data, and one that is not open must name the
 * account it answers from the session.
 */
export const PUBLIC = 'public';
export const SESSION = 'session';
export const OWNED = 'owned';

/** The operations the served set of the contract declares, in the order the generator lists them. */
export function servedOperationIds() {
  const config = readFileSync(SERVED_CONFIG_PATH, 'utf8');
  const listed = config.slice(config.indexOf('include-operation-ids:'));
  return [...listed.matchAll(/^\s+-\s+(\w+)\s*$/gm)].map(([, operationId]) => operationId);
}

/**
 * The paths of the public contract, keyed by the operation identifier declared on them. It reads the
 * two-space indentation of the contract source rather than a second copy of the operation list, so a
 * path that changes changes what the suite calls.
 */
export function declaredOperations() {
  const source = readFileSync(PUBLIC_CONTRACT_PATH, 'utf8');
  const operations = new Map();
  const pathPattern = /^ {2}(\/\S*):\s*$/gm;
  const paths = [...source.matchAll(pathPattern)];
  for (const [index, [, path]] of paths.entries()) {
    const start = paths[index].index;
    const end = index + 1 < paths.length ? paths[index + 1].index : source.length;
    const declaration = /^ {4}(get|post|put|patch|delete):[\s\S]*?^ {6}operationId:\s*(\w+)\s*$/gm;
    for (const [, method, operationId] of source.slice(start, end).matchAll(declaration)) {
      operations.set(operationId, { method: method.toUpperCase(), path });
    }
  }
  return operations;
}

/**
 * accessOf names what each served operation requires. Every operation must appear here: the suite
 * fails on one that does not, which is what stops an operation being served without a decision about
 * who may reach it having been made.
 *
 * The three shapes are the three answers the contract already has for a request that names somebody
 * else's object — a catalog anyone reads, a read of the caller's own account, and an object the
 * caller must hold — and the plan for each owned operation says how a caller reaches it.
 */
const ACCESS = {
  // Health and the catalog are the surfaces a person reads before signing in.
  getHealthLive: { access: PUBLIC },
  getHealthReady: { access: PUBLIC },
  getVehicles: { access: PUBLIC },
  getVehicle: { access: PUBLIC, absent: ABSENT_VEHICLE_IDENTIFIER },
  getZones: { access: PUBLIC },
  getTariffs: { access: PUBLIC },
  getPublicEvents: { access: PUBLIC },

  // An account is created and proven without a session, so these three are reachable by anyone. A
  // sign-out acts on the session the caller presented and on nothing else.
  register: { access: PUBLIC },
  login: { access: PUBLIC },
  logout: { access: PUBLIC },

  // A read of the caller's own account or of the collections derived from it. The owner is the
  // session's account and no parameter selects it.
  getMe: { access: SESSION },
  getCurrentRental: { access: SESSION },
  getRides: { access: SESSION },
  getInvoices: { access: SESSION },
  getNotifications: { access: SESSION },
  getPrivateEvents: { access: SESSION },

  // Operations on one object, which the caller must hold. Each states how a check reaches a rental,
  // an invoice or a notification of its own to aim a foreign request at.
  reserve: { access: SESSION },
  cancelRental: { access: OWNED, object: 'rental', command: true },
  startRental: { access: OWNED, object: 'rental', command: true },
  pauseRental: { access: OWNED, object: 'rental', command: true },
  resumeRental: { access: OWNED, object: 'rental', command: true },
  finishRental: { access: OWNED, object: 'rental', command: true },
  payInvoice: { access: OWNED, object: 'invoice', command: true },
  getInvoice: { access: OWNED, object: 'invoice' },
  readNotification: { access: OWNED, object: 'notification', command: true },
};

/**
 * The matrix of the served set: one row per declared operation, with the path a request is sent to
 * and what reaching it requires. It fails when the contract and this table disagree in either
 * direction, so neither a new operation nor a removed one can pass unnoticed.
 */
export function accessMatrix() {
  const declared = declaredOperations();
  const served = servedOperationIds();
  assert.deepEqual(
    Object.keys(ACCESS).sort(),
    [...served].sort(),
    'the access matrix and the served operations of the contract disagree',
  );

  return served.map((operationId) => {
    const operation = declared.get(operationId);
    assert.ok(operation, `the served operation ${operationId} is not declared in the public contract`);
    return { operationId, ...operation, ...ACCESS[operationId] };
  });
}

/** The path of one operation with an identifier put in the place the contract templates. */
export function pathFor(operation, identifier) {
  if (!TEMPLATE_SEGMENT.test(operation.path)) return operation.path;
  return operation.path.replace(TEMPLATE_SEGMENT, identifier);
}

/**
 * The identifiers that may never be read through another account. The contract declares both kinds
 * in its own document, and the placeholder a malformed request carries proves which one an operation
 * refuses by.
 */
export const OTHER_ACCOUNT_IDENTIFIER = {
  rental: ABSENT_IDENTIFIER,
  invoice: ABSENT_IDENTIFIER,
  notification: ABSENT_IDENTIFIER,
};

/** One request of the matrix, sent the way the operation requires. */
export function sendTo(operation, identifier, options = {}) {
  const { account, key, body, vehicleId } = options;
  return call(pathFor(operation, identifier), {
    method: operation.method,
    body: operation.command ? (body ?? commandBodyOf(operation, vehicleId)) : undefined,
    cookie: account?.cookie,
    csrfToken: operation.command ? account?.csrfToken : undefined,
    headers: operation.command ? { 'Idempotency-Key': key } : {},
  });
}

/**
 * The body one command of the matrix needs to be a request the contract accepts. A reservation names
 * the vehicle it takes and every other command carries nothing, so the only shape stated here is the
 * one the contract states for a reservation.
 */
function commandBodyOf(operation, vehicleId) {
  return operation.operationId === 'reserve' ? { vehicle_id: vehicleId } : undefined;
}

/**
 * A machine-readable rendering of an answer, for comparing two of them field by field. The request
 * identifier is masked because two requests are never the same request: what is compared is the shape
 * of the answer — its status, its code and its message — and never the label of one exchange.
 */
export function answerShape(response) {
  const body = response.json ?? response.text;
  if (body === null || typeof body !== 'object') return { status: response.status, body };
  return { status: response.status, body: { ...body, request_id: 'masked' } };
}

/** The operations of the matrix that act on an object of a kind. */
export function operationsOn(object) {
  return accessMatrix().filter((operation) => operation.object === object);
}
